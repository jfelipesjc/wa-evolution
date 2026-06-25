package api

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newOutboundServer wires a Server with a fake backend and a configured (enabled)
// chatwoot provider for "bot1", with the echo-race sleep zeroed for fast tests.
func newOutboundServer(t *testing.T) (*Server, *fakeBackend) {
	t.Helper()
	chatwootWebhookDelay = 0
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	srv := New(Options{APIKey: testKey, Backend: fb, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
	})
	return srv, fb
}

// agentReply builds a minimal outgoing agent-reply webhook body for chatId.
func agentReply(chatId, content, sourceID string) map[string]any {
	return map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"content":      content,
		"id":           42,
		"conversation": map[string]any{
			"id": 7,
			"meta": map[string]any{
				"sender": map[string]any{"identifier": chatId},
			},
			"messages": []any{
				map[string]any{"source_id": sourceID},
			},
		},
	}
}

func TestChatwootWebhook_AgentTextReply(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	rec := do(t, h, "POST", "/chatwoot/webhook/bot1", "", agentReply("5512981201631", "ola mundo", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() == "" || !contains(rec.Body.String(), `"bot"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 1 {
		t.Fatalf("texts = %d, want 1", len(fb.texts))
	}
	got := fb.texts[0]
	if got.jid != "5512981201631@s.whatsapp.net" {
		t.Fatalf("jid = %q", got.jid)
	}
	if got.text != "ola mundo" {
		t.Fatalf("text = %q", got.text)
	}
}

func TestChatwootWebhook_EchoGuardDropsWAID(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	do(t, h, "POST", "/chatwoot/webhook/bot1", "", agentReply("5512981201631", "echo", "WAID:ABC123"))

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 0 {
		t.Fatalf("WAID echo was not dropped: texts = %d", len(fb.texts))
	}
}

func TestChatwootWebhook_PrivateDropped(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	body := agentReply("5512981201631", "internal note", "")
	body["private"] = true
	do(t, h, "POST", "/chatwoot/webhook/bot1", "", body)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 0 {
		t.Fatalf("private note was forwarded: texts = %d", len(fb.texts))
	}
}

func TestChatwootWebhook_MarkdownTranslate(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	do(t, h, "POST", "/chatwoot/webhook/bot1", "",
		agentReply("5512981201631", "say **bold** and *italic* and ~~strike~~ and `code`", ""))

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 1 {
		t.Fatalf("texts = %d", len(fb.texts))
	}
	want := "say *bold* and _italic_ and ~strike~ and ```code```"
	if fb.texts[0].text != want {
		t.Fatalf("markdown:\n got %q\nwant %q", fb.texts[0].text, want)
	}
}

func TestChatwootWebhook_Attachment(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	// serve the media file.
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer media.Close()

	body := map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"content":      "a caption",
		"id":           42,
		"conversation": map[string]any{
			"id":   7,
			"meta": map[string]any{"sender": map[string]any{"identifier": "5512981201631"}},
			"messages": []any{
				map[string]any{
					"source_id": "",
					"attachments": []any{
						map[string]any{"data_url": media.URL + "/file.png", "file_type": "image"},
					},
				},
			},
		},
	}
	do(t, h, "POST", "/chatwoot/webhook/bot1", "", body)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.medias) != 1 {
		t.Fatalf("medias = %d, want 1", len(fb.medias))
	}
	m := fb.medias[0]
	if m.jid != "5512981201631@s.whatsapp.net" {
		t.Fatalf("jid = %q", m.jid)
	}
	if m.media.Kind != "image" {
		t.Fatalf("kind = %q, want image", m.media.Kind)
	}
	if string(m.media.Data) != "PNGDATA" {
		t.Fatalf("data = %q", m.media.Data)
	}
	if m.media.Caption != "a caption" {
		t.Fatalf("caption = %q", m.media.Caption)
	}
	if m.media.FileName != "file.png" {
		t.Fatalf("fileName = %q", m.media.FileName)
	}
	if len(fb.texts) != 0 {
		t.Fatalf("attachment also sent a text: %d", len(fb.texts))
	}
}

func TestChatwootWebhook_Template(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	body := map[string]any{
		"event":        "message_created",
		"message_type": "template",
		"content":      "line1\r\nline2",
		"conversation": map[string]any{
			"id":       7,
			"meta":     map[string]any{"sender": map[string]any{"identifier": "5512981201631"}},
			"messages": []any{map[string]any{"source_id": ""}},
		},
	}
	do(t, h, "POST", "/chatwoot/webhook/bot1", "", body)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 1 || fb.texts[0].text != "line1\nline2" {
		t.Fatalf("template send = %#v", fb.texts)
	}
}

func TestChatwootWebhook_MessageUpdatedNonDeletionIgnored(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	body := agentReply("5512981201631", "edited text", "")
	body["event"] = "message_updated"
	// no content_attributes.deleted -> ignored.
	do(t, h, "POST", "/chatwoot/webhook/bot1", "", body)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 0 {
		t.Fatalf("non-deletion edit was forwarded: %d", len(fb.texts))
	}
}

func TestChatwootWebhook_PhoneNumberFallback(t *testing.T) {
	srv, fb := newOutboundServer(t)
	h := srv.Handler()

	body := map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"content":      "hi",
		"conversation": map[string]any{
			"id":       7,
			"meta":     map[string]any{"sender": map[string]any{"phone_number": "+5512981201631"}},
			"messages": []any{map[string]any{"source_id": ""}},
		},
	}
	do(t, h, "POST", "/chatwoot/webhook/bot1", "", body)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.texts) != 1 || fb.texts[0].jid != "5512981201631@s.whatsapp.net" {
		t.Fatalf("phone_number fallback jid wrong: %#v", fb.texts)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
