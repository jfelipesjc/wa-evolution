package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// chatwoot_inbound.go — Phase 2b: the WhatsApp->Chatwoot inbound pipeline.
// Conversation/message create helpers on *chatwootClient plus the backend-neutral
// orchestration seam Server.chatwootHandleInbound that the event pump calls per
// inbound message. Mirrors Evolution's chatwoot.service.ts createConversation /
// createMessage / sendData. Phase 4 (ads/reactions/interactive, reply linkage,
// history import, label tagging) is intentionally left as TODOs.

// cwConversation is the subset of a Chatwoot conversation we read.
type cwConversation struct {
	ID      int    `json:"id"`
	InboxID int    `json:"inbox_id"`
	Status  string `json:"status"`
}

// GetInboxByName returns the inbox whose name matches, or nil if absent.
func (cw *chatwootClient) GetInboxByName(ctx context.Context, name string) (*cwInbox, error) {
	inboxes, err := cw.ListInboxes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range inboxes {
		if inboxes[i].Name == name {
			return &inboxes[i], nil
		}
	}
	return nil, nil
}

// ListContactConversations returns the conversations attached to a contact.
func (cw *chatwootClient) ListContactConversations(ctx context.Context, contactID int) ([]cwConversation, error) {
	var resp struct {
		Payload []cwConversation `json:"payload"`
	}
	err := cw.do(ctx, http.MethodGet, cw.acctPath(fmt.Sprintf("/contacts/%d/conversations", contactID)), nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Payload, nil
}

// GetConversation fetches a single conversation (used to verify a cached id).
func (cw *chatwootClient) GetConversation(ctx context.Context, id int) (*cwConversation, error) {
	var c cwConversation
	if err := cw.do(ctx, http.MethodGet, cw.acctPath(fmt.Sprintf("/conversations/%d", id)), nil, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateConversation creates a conversation. contact_id/inbox_id are sent as
// STRINGS (per Evolution); status:"pending" only when pending is true.
func (cw *chatwootClient) CreateConversation(ctx context.Context, contactID, inboxID int, pending bool) (int, error) {
	body := map[string]any{
		"contact_id": strconv.Itoa(contactID),
		"inbox_id":   strconv.Itoa(inboxID),
	}
	if pending {
		body["status"] = "pending"
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := cw.do(ctx, http.MethodPost, cw.acctPath("/conversations"), body, &resp); err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// CreateTextMessage posts a JSON text message and returns its Chatwoot id. When
// inReplyTo != 0 it sets content_attributes.in_reply_to to thread the message as a
// reply to that Chatwoot message id.
func (cw *chatwootClient) CreateTextMessage(ctx context.Context, convID int, content, messageType, sourceID string, inReplyTo int) (int, error) {
	body := map[string]any{
		"content":      content,
		"message_type": messageType,
		"private":      false,
		"source_id":    sourceID,
	}
	if inReplyTo != 0 {
		body["content_attributes"] = map[string]any{"in_reply_to": inReplyTo}
	}
	var resp struct {
		ID int `json:"id"`
	}
	err := cw.do(ctx, http.MethodPost, cw.acctPath(fmt.Sprintf("/conversations/%d/messages", convID)), body, &resp)
	if err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// CreateMediaMessage posts a multipart message with one attachment and returns
// its Chatwoot id. content is optional (sent only when non-empty). When
// inReplyTo != 0 it sets the content_attributes form field to thread the message
// as a reply to that Chatwoot message id.
func (cw *chatwootClient) CreateMediaMessage(ctx context.Context, convID int, content, messageType, sourceID, fileName string, data []byte, inReplyTo int) (int, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if content != "" {
		_ = mw.WriteField("content", content)
	}
	_ = mw.WriteField("message_type", messageType)
	if sourceID != "" {
		_ = mw.WriteField("source_id", sourceID)
	}
	if inReplyTo != 0 {
		_ = mw.WriteField("content_attributes", fmt.Sprintf(`{"in_reply_to":%d}`, inReplyTo))
	}
	fw, err := mw.CreateFormFile("attachments[]", fileName)
	if err != nil {
		return 0, err
	}
	if _, err := fw.Write(data); err != nil {
		return 0, err
	}
	if err := mw.Close(); err != nil {
		return 0, err
	}

	url := cw.acctPath(fmt.Sprintf("/conversations/%d/messages", convID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("api_access_token", cw.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := cw.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("chatwoot media POST %s: %d", url, resp.StatusCode)
	}
	var out struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// --- the inbound orchestration seam ---

// InboundMessage is a backend-neutral inbound WhatsApp message for the Chatwoot bridge.
type InboundMessage struct {
	JID      string // remoteJid e.g. "5512...@s.whatsapp.net" or a group/@lid jid
	FromMe   bool
	MsgID    string // WA message id (key.id)
	PushName string
	Text     string // extracted text/caption ("" if media-only)
	IsMedia  bool
	Mimetype string
	FileName string
	Download func() ([]byte, string, error) // lazy media fetch -> (bytes, mimetype); may be nil
	// QuotedWAID is the WA stanza id of the message this one quotes ("" if none).
	QuotedWAID string
}

// chatwootInboundCache caches resolved contact/conversation ids per (instance,jid)
// to avoid refetching on every message. Correctness still comes first: the
// conversation id is verified against Chatwoot before reuse.
type chatwootInboundCache struct {
	mu      sync.Mutex
	contact map[string]int // key: instance|jid
	conv    map[string]int // key: instance|jid
}

func newChatwootInboundCache() *chatwootInboundCache {
	return &chatwootInboundCache{contact: map[string]int{}, conv: map[string]int{}}
}

var deviceSuffixRe = regexp.MustCompile(`:\d+`)

// normalizeJid derives the phone digits used for lookups from a JID. @lid jids are
// passed through unchanged (lids are not phone numbers); otherwise the Baileys
// device suffix (":\d+") and the "@..." part are stripped.
func normalizeJid(jid string) string {
	if jid == "" {
		return ""
	}
	if strings.Contains(jid, "@lid") {
		return jid
	}
	jid = deviceSuffixRe.ReplaceAllString(jid, "")
	return strings.SplitN(jid, "@", 2)[0]
}

// HandleChatwootInbound is the exported entry the host event pump calls for each
// inbound WhatsApp message; it delegates to the bridge (no-op if chatwoot is not
// configured/enabled for the instance).
func (s *Server) HandleChatwootInbound(ctx context.Context, instance string, m InboundMessage) {
	s.chatwootHandleInbound(ctx, instance, m)
}

// chatwootHandleInbound bridges one inbound WhatsApp message to Chatwoot for an
// instance, using its stored config. No-op (nil) if chatwoot disabled/absent or
// the JID is ignored (ignoreJids, groups, status@broadcast).
func (s *Server) chatwootHandleInbound(ctx context.Context, instance string, m InboundMessage) {
	cfg, ok := s.chatwoot.get(instance)
	if !ok || !cfg.Enabled {
		return
	}

	// ignoreJids gate.
	if m.JID == "status@broadcast" {
		return
	}
	var ignoreGroups, ignoreContacts bool
	for _, j := range cfg.IgnoreJids {
		switch j {
		case "@g.us":
			ignoreGroups = true
		case "@s.whatsapp.net":
			ignoreContacts = true
		}
		if j == m.JID {
			return
		}
	}
	if ignoreGroups && strings.HasSuffix(m.JID, "@g.us") {
		return
	}
	if ignoreContacts && strings.HasSuffix(m.JID, "@s.whatsapp.net") {
		return
	}
	// Groups are unsupported here (Phase 4); drop them defensively even if not
	// listed in ignoreJids.
	if strings.HasSuffix(m.JID, "@g.us") {
		return
	}

	// Nothing to deliver?
	if !m.IsMedia && m.Text == "" {
		return
	}

	cw := newChatwootClient(cfg)

	inbox, err := cw.GetInboxByName(ctx, cfg.NameInbox)
	if err != nil {
		s.logger.Printf("chatwoot inbound %s: list inboxes: %v", instance, err)
		return
	}
	if inbox == nil {
		s.logger.Printf("chatwoot inbound %s: inbox %q not found", instance, cfg.NameInbox)
		return
	}

	digits := normalizeJid(m.JID)

	contactID, err := s.resolveContact(ctx, cw, instance, m, inbox.ID, digits)
	if err != nil || contactID == 0 {
		if err != nil {
			s.logger.Printf("chatwoot inbound %s: resolve contact: %v", instance, err)
		}
		return
	}

	convID, err := s.resolveConversation(ctx, cw, instance, m.JID, contactID, inbox.ID, cfg.ConversationPending)
	if err != nil || convID == 0 {
		if err != nil {
			s.logger.Printf("chatwoot inbound %s: resolve conversation: %v", instance, err)
		}
		return
	}

	messageType := "incoming"
	if m.FromMe {
		messageType = "outgoing"
	}
	sourceID := "WAID:" + m.MsgID

	// Quoted/reply linkage: if this WA message quotes an earlier one we already
	// bridged, thread the Chatwoot message under it via in_reply_to.
	var inReplyTo int
	if m.QuotedWAID != "" {
		if id, ok := s.chatwootMsgs.chatwootIDForWA(instance, m.QuotedWAID); ok {
			inReplyTo = id
		}
	}

	var chatwootID int
	if m.IsMedia && m.Download != nil {
		data, mime, derr := m.Download()
		if derr != nil {
			s.logger.Printf("chatwoot inbound %s: media download: %v", instance, derr)
			return
		}
		fileName := m.FileName
		if fileName == "" {
			fileName = mediaFileName(mime)
		}
		id, err := cw.CreateMediaMessage(ctx, convID, m.Text, messageType, sourceID, fileName, data, inReplyTo)
		if err != nil {
			s.logger.Printf("chatwoot inbound %s: create media message: %v", instance, err)
			return
		}
		chatwootID = id
	} else if m.Text != "" {
		id, err := cw.CreateTextMessage(ctx, convID, m.Text, messageType, sourceID, inReplyTo)
		if err != nil {
			s.logger.Printf("chatwoot inbound %s: create text message: %v", instance, err)
			return
		}
		chatwootID = id
	} else {
		return
	}

	// Record the WA<->Chatwoot id mapping so this message can be quoted later
	// (from either direction).
	s.chatwootMsgs.record(instance, chatwootID, waMsgRef{
		WAID:      m.MsgID,
		RemoteJID: m.JID,
		FromMe:    m.FromMe,
		Text:      m.Text,
	})
	// TODO(phase 4+): reactions, interactive (PIX) buttons, CTWA ads.
}

// resolveContact finds-or-creates the Chatwoot contact for an inbound message,
// using a small per-(instance,jid) cache. The Brazilian 9th-digit merge happens
// inside FindContact.
func (s *Server) resolveContact(ctx context.Context, cw *chatwootClient, instance string, m InboundMessage, inboxID int, digits string) (int, error) {
	key := instance + "|" + m.JID
	s.chatwootCache.mu.Lock()
	cached := s.chatwootCache.contact[key]
	s.chatwootCache.mu.Unlock()
	if cached != 0 {
		return cached, nil
	}

	contact, err := cw.FindContact(ctx, digits)
	if err != nil {
		return 0, err
	}
	if contact == nil {
		contact, err = cw.CreateContact(ctx, inboxID, m.PushName, m.JID, "")
		if err != nil {
			return 0, err
		}
		if contact == nil {
			return 0, nil
		}
	}

	s.chatwootCache.mu.Lock()
	s.chatwootCache.contact[key] = contact.ID
	s.chatwootCache.mu.Unlock()
	return contact.ID, nil
}

// resolveConversation reuses a non-resolved conversation in the inbox or creates a
// new one (reopenConversation is OFF). Cached per (instance,jid) and verified
// against Chatwoot before reuse.
func (s *Server) resolveConversation(ctx context.Context, cw *chatwootClient, instance, jid string, contactID, inboxID int, pending bool) (int, error) {
	key := instance + "|" + jid
	s.chatwootCache.mu.Lock()
	cached := s.chatwootCache.conv[key]
	s.chatwootCache.mu.Unlock()
	if cached != 0 {
		if conv, err := cw.GetConversation(ctx, cached); err == nil && conv != nil && conv.Status != "resolved" {
			return cached, nil
		}
		s.chatwootCache.mu.Lock()
		delete(s.chatwootCache.conv, key)
		s.chatwootCache.mu.Unlock()
	}

	convs, err := cw.ListContactConversations(ctx, contactID)
	if err != nil {
		return 0, err
	}
	for _, c := range convs {
		// reopenConversation OFF: only reuse a non-resolved conversation.
		if c.InboxID == inboxID && c.Status != "resolved" {
			s.chatwootCache.mu.Lock()
			s.chatwootCache.conv[key] = c.ID
			s.chatwootCache.mu.Unlock()
			return c.ID, nil
		}
	}

	convID, err := cw.CreateConversation(ctx, contactID, inboxID, pending)
	if err != nil {
		return 0, err
	}
	s.chatwootCache.mu.Lock()
	s.chatwootCache.conv[key] = convID
	s.chatwootCache.mu.Unlock()
	return convID, nil
}

// mediaFileName builds a fallback filename from a mimetype.
func mediaFileName(mime string) string {
	ext := "bin"
	if i := strings.Index(mime, "/"); i >= 0 && i+1 < len(mime) {
		sub := mime[i+1:]
		if j := strings.IndexAny(sub, ";+"); j >= 0 {
			sub = sub[:j]
		}
		if sub != "" {
			ext = sub
		}
	}
	return "file." + ext
}
