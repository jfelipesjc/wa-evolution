package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	wa "github.com/felipeleal/wa-go/wa"
)

// httpDoer is the minimal http.Client surface the dispatcher needs; tests inject
// a fake or an httptest-backed *http.Client.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// webhookDispatcher holds the per-instance webhook URLs and POSTs Evolution-shaped
// envelopes to them. It is the outbound (receive -> Chatwoot/worker) side.
type webhookDispatcher struct {
	mu     sync.RWMutex
	urls   map[string]string // instance -> webhook URL
	client httpDoer
	logger *log.Logger
}

func newWebhookDispatcher(client httpDoer, logger *log.Logger) *webhookDispatcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &webhookDispatcher{
		urls:   make(map[string]string),
		client: client,
		logger: logger,
	}
}

// set registers (or replaces) the webhook URL for an instance.
func (d *webhookDispatcher) set(instance, url string) {
	d.mu.Lock()
	d.urls[instance] = url
	d.mu.Unlock()
}

// remove drops an instance's webhook URL.
func (d *webhookDispatcher) remove(instance string) {
	d.mu.Lock()
	delete(d.urls, instance)
	d.mu.Unlock()
}

// url returns the configured webhook URL for an instance (empty if none).
func (d *webhookDispatcher) url(instance string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.urls[instance]
}

// dispatch POSTs an Evolution envelope to the instance's webhook URL if one is
// configured and the event maps to a webhook. Delivery is best-effort: errors
// are logged and dropped so a slow consumer never blocks the event pump.
func (d *webhookDispatcher) dispatch(ctx context.Context, instance string, ev wa.Event) {
	url := d.url(instance)
	if url == "" {
		return
	}
	evt, data, ok := toEvolutionEvent(ev)
	if !ok {
		return
	}
	env := webhookEnvelope{
		Event:    evt,
		Instance: instance,
		Data:     data,
		DateTime: time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(env)
	if err != nil {
		d.logger.Printf("api/webhook: marshal %s: %v", evt, err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		d.logger.Printf("api/webhook: new request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Printf("api/webhook: POST %s (%s): %v", url, evt, err)
		return
	}
	// Drain+close so the transport can reuse the connection.
	_ = resp.Body.Close()
}

// RunEventPump drains the manager's aggregated event stream until ctx is done,
// dispatching each event to the owning instance's webhook and (for messages)
// feeding the per-instance ChatStore via the supplied feed callback. feed may be
// nil. It returns when the events channel closes or ctx is cancelled.
func RunEventPump(ctx context.Context, mgr *wa.Manager, d *webhookDispatcher, feed func(instance string, ev wa.Event)) {
	events := mgr.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ie, ok := <-events:
			if !ok {
				return
			}
			if feed != nil {
				feed(ie.Name, ie.Event)
			}
			d.dispatch(ctx, ie.Name, ie.Event)
		}
	}
}

// toEvolutionEvent maps a wa.Event to the Evolution event name + data shape.
// The second return is the JSON-serializable `data` object; the bool is false
// for events that do not produce a webhook.
func toEvolutionEvent(ev wa.Event) (string, interface{}, bool) {
	switch e := ev.(type) {
	case wa.MessageEvent:
		return "messages.upsert", messageUpsertData(e), true

	case wa.ReceiptUpdateEvent:
		return "messages.update", map[string]interface{}{
			"keyId":       firstOf(e.For),
			"keyIds":      e.For,
			"remoteJid":   e.From,
			"participant": e.Participant,
			"status":      string(e.Type),
		}, true
	case wa.ReceiptEvent:
		return "messages.update", map[string]interface{}{
			"keyId":     e.ID,
			"remoteJid": e.From,
			"status":    e.Type,
		}, true

	case wa.QREvent:
		b64, dataURI := qrPNGBase64(e.Code)
		return "qrcode.updated", map[string]interface{}{
			"qrcode": map[string]interface{}{
				"code":   e.Code,
				"base64": dataURI,
				"png":    b64,
			},
		}, true

	case wa.LoggedInEvent:
		return "connection.update", map[string]interface{}{"state": "open"}, true
	case wa.PairSuccessEvent:
		return "connection.update", map[string]interface{}{"state": "open", "jid": e.JID}, true
	case wa.DisconnectedEvent:
		return "connection.update", map[string]interface{}{"state": "close", "statusReason": e.Reason}, true

	case wa.PresenceEvent:
		return "presence.update", map[string]interface{}{
			"id": e.From,
			"presences": map[string]interface{}{
				e.From: map[string]interface{}{"lastKnownPresence": e.State},
			},
		}, true

	case wa.GroupParticipantsUpdateEvent:
		return "group-participants.update", map[string]interface{}{
			"id":           e.GroupJID,
			"action":       string(e.Action),
			"participants": e.Participants,
			"author":       e.By,
		}, true

	default:
		return "", nil, false
	}
}

// messageUpsertData builds the Evolution messages.upsert data object from a
// MessageEvent. The message body is placed under message.conversation for text,
// preserving Evolution's shape; richer media metadata is summarized.
func messageUpsertData(e wa.MessageEvent) map[string]interface{} {
	remoteJID := e.From
	key := map[string]interface{}{
		"remoteJid": remoteJID,
		"fromMe":    false,
		"id":        e.ID,
	}
	if e.IsGroup && e.Sender != "" {
		key["participant"] = e.Sender
	}
	msg := map[string]interface{}{}
	if e.Text != "" {
		msg["conversation"] = e.Text
	}
	if e.Media != nil {
		msg["mediaType"] = string(e.Media.Kind)
		if e.Media.Caption != "" {
			msg["caption"] = e.Media.Caption
		}
	}
	return map[string]interface{}{
		"key":              key,
		"pushName":         e.PushName,
		"message":          msg,
		"messageType":      string(e.Type),
		"messageTimestamp": e.Timestamp,
	}
}

func firstOf(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

// feedChatStore is the standard feed callback for RunEventPump: it consumes
// MessageEvents (and other store-relevant events) into the per-instance
// ChatStore resolved by lookup. lookup returns nil for unknown instances.
func feedChatStore(lookup func(instance string) *wa.ChatStore) func(string, wa.Event) {
	return func(instance string, ev wa.Event) {
		cs := lookup(instance)
		if cs == nil {
			return
		}
		cs.Consume(ev)
	}
}
