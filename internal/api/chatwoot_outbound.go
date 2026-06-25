package api

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// chatwoot_outbound.go — Phase 3: the Chatwoot agent-reply -> WhatsApp bridge
// (Evolution's receiveWebhook cluster). The Chatwoot install POSTs agent replies
// to /chatwoot/webhook/{instance}; this turns them into WhatsApp sends via the
// backend. Mirrors chatwoot.service.ts receiveWebhook / sendAttachment /
// onSendMessageError. Scoped to the user's config (signMsg=false,
// reopenConversation=false, import OFF, accountId='1') but kept correct generally.

// chatwootWebhookDelay is the echo-race guard sleep applied at the top of webhook
// handling (spec §0/§2). A package var so tests can zero it.
var chatwootWebhookDelay = 500 * time.Millisecond

// cwWebhook is the defensively-parsed subset of the Chatwoot webhook payload.
type cwWebhook struct {
	Event             string `json:"event"`
	MessageType       string `json:"message_type"`
	Private           bool   `json:"private"`
	Content           string `json:"content"`
	ID                int    `json:"id"`
	Status            string `json:"status"`
	ContentAttributes struct {
		Deleted   bool `json:"deleted"`
		InReplyTo int  `json:"in_reply_to"`
	} `json:"content_attributes"`
	Inbox struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"inbox"`
	Conversation *struct {
		ID           int `json:"id"`
		ContactInbox struct {
			SourceID string `json:"source_id"`
		} `json:"contact_inbox"`
		Meta struct {
			Sender struct {
				Identifier  string `json:"identifier"`
				PhoneNumber string `json:"phone_number"`
			} `json:"sender"`
		} `json:"meta"`
		Messages []cwWebhookMessage `json:"messages"`
	} `json:"conversation"`
	Sender struct {
		Name string `json:"name"`
	} `json:"sender"`
}

type cwWebhookMessage struct {
	SourceID string `json:"source_id"`
	Sender   struct {
		AvailableName string `json:"available_name"`
	} `json:"sender"`
	Attachments []cwWebhookAttachment `json:"attachments"`
}

type cwWebhookAttachment struct {
	DataURL  string `json:"data_url"`
	FileType string `json:"file_type"`
}

// chatwootMarkdownToWA translates Chatwoot markdown to WhatsApp formatting.
//
// Spec order (with JS lookarounds): *x*->_x_, **x**->*x*, ~~x~~->~x~, `x`->```x```.
// Go's regexp has no lookarounds, so bold (`**`) cannot be skipped by a plain
// `\*...\*` italic pass. DEVIATION: we run the bold pass FIRST into a sentinel,
// then italic, then restore the sentinel to a single `*`. This yields the same
// result as the lookaround-guarded JS ordering (bold survives the italic pass)
// for normal input. The one observable difference from the JS regexes is that JS
// requires non-space at the boundaries (`(?!\s)...(?<!\s)`); we keep that via the
// boundary classes below, and we do not match empty/space-only spans.
func chatwootMarkdownToWA(s string) string {
	if s == "" {
		return s
	}
	// 1) bold **x** -> sentinel-wrapped (protect from the italic pass).
	s = reBold.ReplaceAllString(s, "\x00${1}\x00")
	// 2) italic *x* -> _x_ (now no `**` remain to confuse it).
	s = reItalic.ReplaceAllString(s, "_${1}_")
	// 3) restore bold sentinels to WA bold *x*.
	s = strings.ReplaceAll(s, "\x00", "*")
	// 4) strikethrough ~~x~~ -> ~x~.
	s = reStrike.ReplaceAllString(s, "~${1}~")
	// 5) inline code `x` -> ```x``` (WA code block).
	s = reCode.ReplaceAllString(s, "```${1}```")
	return s
}

var (
	// non-space-bounded spans, mirroring JS (?!\s)...(?<!\s).
	reBold   = regexp.MustCompile(`\*\*([^\s*](?:[^\n*]*[^\s*])?)\*\*`)
	reItalic = regexp.MustCompile(`\*([^\s*](?:[^\n*]*[^\s*])?)\*`)
	reStrike = regexp.MustCompile(`~~([^\s~](?:[^\n~]*[^\s~])?)~~`)
	reCode   = regexp.MustCompile("`([^\\s`](?:[^`*]*[^\\s`])?)`")
)

// handleChatwootWebhook: POST /chatwoot/webhook/{instance} (NO apikey — Chatwoot
// posts here). Forwards agent replies to WhatsApp. Always returns 200 {"message":
// "bot"}; a panic is recovered so it can never 500.
func (s *Server) handleChatwootWebhook(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Printf("chatwoot webhook %s: panic: %v", instance, rec)
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"message": "bot"})
	}()

	cfg, ok := s.chatwoot.get(instance)
	if !ok || !cfg.Enabled {
		io.Copy(io.Discard, io.LimitReader(r.Body, 4<<20))
		return
	}

	var body cwWebhook
	if !s.tryDecodeJSON(r, &body) {
		// Malformed/empty body: fall through to the deferred 200 ack.
		return
	}

	// 500ms echo-race guard.
	if chatwootWebhookDelay > 0 {
		time.Sleep(chatwootWebhookDelay)
	}

	s.chatwootProcessWebhook(r.Context(), instance, cfg, body)
}

// chatwootProcessWebhook runs the receiveWebhook branch logic. It never returns an
// error — failures are logged or posted back as private notes.
func (s *Server) chatwootProcessWebhook(ctx context.Context, instance string, cfg chatwootConfig, body cwWebhook) {
	// Early bail conditions (spec §2.3).
	if body.Conversation == nil {
		return
	}
	if body.Private {
		return
	}
	if body.Event == "message_updated" && !body.ContentAttributes.Deleted {
		return
	}

	// chatId = identifier OR phone_number (strip leading '+'); bare digits.
	chatId := body.Conversation.Meta.Sender.Identifier
	if chatId == "" {
		chatId = strings.TrimPrefix(body.Conversation.Meta.Sender.PhoneNumber, "+")
	}

	// markdown-translate the content (Chatwoot -> WhatsApp).
	messageReceived := chatwootMarkdownToWA(body.Content)

	cw := newChatwootClient(cfg)

	// Template branch (spec §2.11): plain-text send of body.content.
	if body.MessageType == "template" && body.Event == "message_created" {
		text := strings.ReplaceAll(body.Content, "\r\n", "\n")
		if chatId != "" && text != "" {
			jid := normalizeJID(chatId)
			if _, err := s.backend.SendText(ctx, instance, jid, text); err != nil {
				s.logger.Printf("chatwoot webhook %s: template send: %v", instance, err)
			}
		}
		return
	}

	// MAIN outbound send branch.
	if !(body.MessageType == "outgoing" && len(body.Conversation.Messages) > 0 && chatId != "123456") {
		return
	}

	// ECHO GUARD: messages[0].source_id starting with "WAID:" came FROM WhatsApp.
	if strings.HasPrefix(body.Conversation.Messages[0].SourceID, "WAID:") {
		return
	}

	// signMsg=false -> formatText = messageReceived. Implement the sign branch for
	// correctness (no-op for this user).
	senderName := body.Conversation.Messages[0].Sender.AvailableName
	if senderName == "" {
		senderName = body.Sender.Name
	}
	formatText := messageReceived
	if senderName != "" {
		delimiter := "\n"
		if cfg.SignDelimiter != "" {
			delimiter = strings.ReplaceAll(cfg.SignDelimiter, `\n`, "\n")
		}
		var parts []string
		if cfg.SignMsg {
			parts = append(parts, "*"+senderName+":*")
		}
		parts = append(parts, messageReceived)
		formatText = strings.Join(parts, delimiter)
	}

	jid := normalizeJID(chatId)

	// Note on the second-layer echo prevention (isIntegration flag in Evolution):
	// our inbound bridge only mirrors RECEIVED wa.MessageEvents into Chatwoot, never
	// our own sends, so an outgoing send here is never re-pushed into Chatwoot. We
	// therefore do not need an isIntegration/suppress-mirror flag on the send.
	for _, msg := range body.Conversation.Messages {
		if len(msg.Attachments) > 0 {
			for _, att := range msg.Attachments {
				if err := s.chatwootSendAttachment(ctx, instance, jid, att, formatText); err != nil {
					s.logger.Printf("chatwoot webhook %s: send attachment: %v", instance, err)
					s.chatwootPostError(ctx, cw, body.Conversation.ID, err)
				}
			}
			continue
		}
		// text-only message
		if formatText == "" {
			continue
		}
		// Quoted reply: if the agent replied to a message we have a WA mapping for,
		// send a WhatsApp quote so the citation renders on WhatsApp.
		var (
			waID    string
			sendErr error
		)
		if body.ContentAttributes.InReplyTo != 0 {
			if ref, ok := s.chatwootMsgs.waRefForChatwootID(instance, body.ContentAttributes.InReplyTo); ok {
				waID, sendErr = s.backend.SendTextReply(ctx, instance, jid, formatText, QuotedRef{
					ID:          ref.WAID,
					RemoteJID:   ref.RemoteJID,
					FromMe:      ref.FromMe,
					Text:        ref.Text,
					Participant: ref.Participant,
				})
			} else {
				waID, sendErr = s.backend.SendText(ctx, instance, jid, formatText)
			}
		} else {
			waID, sendErr = s.backend.SendText(ctx, instance, jid, formatText)
		}
		if sendErr != nil {
			s.logger.Printf("chatwoot webhook %s: send text: %v", instance, sendErr)
			s.chatwootPostError(ctx, cw, body.Conversation.ID, sendErr)
			continue
		}
		// Record this agent reply's WA<->Chatwoot id mapping so a future reply to it
		// resolves back to this WhatsApp message.
		s.chatwootMsgs.record(instance, body.ID, waMsgRef{
			WAID:      waID,
			RemoteJID: jid,
			FromMe:    true,
			Text:      formatText,
		})
	}
	// NOTE: local DB id-mapping for media replies and markMessageAsRead are skipped
	// — media replies don't carry a quote in this phase.
}

// chatwootSendAttachment downloads an attachment and forwards it to WhatsApp,
// deriving the media kind from its MIME type. Returns an error so the caller can
// post a private failure note.
func (s *Server) chatwootSendAttachment(ctx context.Context, instance, jid string, att cwWebhookAttachment, caption string) error {
	data, mimetype, err := downloadURL(ctx, att.DataURL)
	if err != nil {
		return err
	}

	kind := mediaKindFromMime(mimetype)
	fileName := fileNameFromURL(att.DataURL)

	if kind == "audio" {
		_, err := s.backend.SendWhatsAppAudio(ctx, instance, jid, data, mimetype)
		return err
	}

	// gif/svg/tiff/tif/dxf/dwg -> force document.
	if kind == "image" && extForcesDocument(fileName) {
		kind = "document"
	}

	m := MediaArg{
		Kind:     kind,
		Data:     data,
		FileName: fileName,
		Mimetype: mimetype,
	}
	if caption != "" {
		m.Caption = caption
	}
	_, err = s.backend.SendMedia(ctx, instance, jid, m)
	return err
}

// chatwootPostError best-effort posts a private failure note back to the Chatwoot
// conversation. Never returns an error (the webhook always 200s).
func (s *Server) chatwootPostError(ctx context.Context, cw *chatwootClient, convID int, sendErr error) {
	if convID == 0 {
		return
	}
	content := "Message not sent."
	if sendErr != nil {
		content = "Message not sent. _" + sendErr.Error() + "_"
	}
	body := map[string]any{
		"content":      content,
		"message_type": "outgoing",
		"private":      true,
	}
	url := cw.acctPath("/conversations/" + itoa(convID) + "/messages")
	if err := cw.do(ctx, http.MethodPost, url, body, nil); err != nil {
		s.logger.Printf("chatwoot: post error note: %v", err)
	}
}

// --- small helpers ---

// downloadURL GETs a URL and returns its bytes and content-type.
func downloadURL(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// mediaKindFromMime maps a MIME type to a WhatsApp media kind.
func mediaKindFromMime(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	default:
		return "document"
	}
}

// extForcesDocument reports whether an image extension must be sent as a document.
func extForcesDocument(fileName string) bool {
	lower := strings.ToLower(fileName)
	for _, ext := range []string{".gif", ".svg", ".tiff", ".tif", ".dxf", ".dwg"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fileNameFromURL returns the last path segment of a URL (query/fragment stripped).
func fileNameFromURL(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndex(u, "/"); i >= 0 {
		u = u[i+1:]
	}
	return u
}

// itoa is a tiny int->string without importing strconv twice; kept local.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
