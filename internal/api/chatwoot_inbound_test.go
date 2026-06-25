package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fullMockChatwoot extends the basic inbox mock with contacts, conversations and
// messages so the inbound pipeline can be exercised end-to-end offline. It records
// the exact payloads it received for assertions.
type fullMockChatwoot struct {
	mu sync.Mutex

	inboxes []cwInbox

	contacts     []cwContact
	nextContact  int
	mergeReqs    []map[string]int
	createReqs   []map[string]any
	filterCalls  int
	createCount  int
	create422jid string // if set and create identifier == this, respond 422

	conversations  []cwConversation
	nextConv       int
	createdConvs   []map[string]any
	listConvCalls  int

	messages       []map[string]any // recorded create-message bodies
	mediaMessages  []recordedMedia
	nextMsg        int
}

type recordedMedia struct {
	Content     string
	MessageType string
	SourceID    string
	FileName    string
	Data        []byte
}

func newFullMock() *fullMockChatwoot {
	return &fullMockChatwoot{nextContact: 200, nextConv: 300, nextMsg: 900}
}

func (m *fullMockChatwoot) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/accounts/{acct}/inbox_list", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": m.inboxes})
	})

	mux.HandleFunc("POST /api/v1/accounts/{acct}/contacts/filter", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Payload []cwFilterClause `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.filterCalls++
		// collect the requested phone values (without +) and identifiers.
		wantPhones := map[string]bool{}
		wantIdent := map[string]bool{}
		for _, c := range body.Payload {
			for _, v := range c.Values {
				if c.AttributeKey == "phone_number" {
					wantPhones[v] = true
				} else if c.AttributeKey == "identifier" {
					wantIdent[v] = true
				}
			}
		}
		var out []cwContact
		for _, c := range m.contacts {
			if wantPhones[strings.TrimPrefix(c.PhoneNumber, "+")] || wantIdent[c.Identifier] {
				out = append(out, c)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": out})
	})

	mux.HandleFunc("GET /api/v1/accounts/{acct}/contacts/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []cwContact
		for _, c := range m.contacts {
			if c.Identifier == q {
				out = append(out, c)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": out})
	})

	mux.HandleFunc("POST /api/v1/accounts/{acct}/contacts", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.createCount++
		m.createReqs = append(m.createReqs, body)
		ident, _ := body["identifier"].(string)
		if m.create422jid != "" && ident == m.create422jid {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "identifier taken"})
			return
		}
		c := cwContact{ID: m.nextContact, Identifier: ident}
		m.nextContact++
		if p, ok := body["phone_number"].(string); ok {
			c.PhoneNumber = p
		}
		if n, ok := body["name"].(string); ok {
			c.Name = n
		}
		m.contacts = append(m.contacts, c)
		// Chatwoot wraps new contact under payload.contact
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": map[string]any{"contact": c}})
	})

	mux.HandleFunc("POST /api/v1/accounts/{acct}/actions/contact_merge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.mergeReqs = append(m.mergeReqs, body)
		base := body["base_contact_id"]
		var survivor cwContact
		for _, c := range m.contacts {
			if c.ID == base {
				survivor = c
			}
		}
		_ = json.NewEncoder(w).Encode(survivor)
	})

	mux.HandleFunc("GET /api/v1/accounts/{acct}/contacts/{id}/conversations", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.listConvCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": m.conversations})
	})

	mux.HandleFunc("GET /api/v1/accounts/{acct}/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, c := range m.conversations {
			_ = c
		}
		_ = json.NewEncoder(w).Encode(cwConversation{ID: 0, Status: "open"})
	})

	mux.HandleFunc("POST /api/v1/accounts/{acct}/conversations", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.createdConvs = append(m.createdConvs, body)
		conv := cwConversation{ID: m.nextConv, Status: "open"}
		m.nextConv++
		m.conversations = append(m.conversations, conv)
		_ = json.NewEncoder(w).Encode(conv)
	})

	mux.HandleFunc("POST /api/v1/accounts/{acct}/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		m.mu.Lock()
		defer m.mu.Unlock()
		id := m.nextMsg
		m.nextMsg++
		if strings.HasPrefix(ct, "multipart/") {
			_, params, _ := mime.ParseMediaType(ct)
			mr := multipart.NewReader(r.Body, params["boundary"])
			rec := recordedMedia{}
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(part)
				switch part.FormName() {
				case "content":
					rec.Content = string(data)
				case "message_type":
					rec.MessageType = string(data)
				case "source_id":
					rec.SourceID = string(data)
				case "attachments[]":
					rec.FileName = part.FileName()
					rec.Data = data
				}
			}
			m.mediaMessages = append(m.mediaMessages, rec)
		} else {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.messages = append(m.messages, body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})

	return mux
}

func newInboundServer(t *testing.T, mock *fullMockChatwoot, cfg chatwootConfig) (*Server, string) {
	t.Helper()
	cwsrv := httptest.NewServer(mock.handler())
	t.Cleanup(cwsrv.Close)
	cfg.URL = cwsrv.URL
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	srv := New(Options{APIKey: testKey, Backend: fb, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", cfg)
	return srv, cwsrv.URL
}

func TestChatwootHandleInbound_CreatesEverything(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
		MergeBrazilContacts: true, IgnoreJids: []string{"@g.us"},
	})

	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID:      "5512981201631@s.whatsapp.net",
		FromMe:   false,
		MsgID:    "ABC123",
		PushName: "Felipe",
		Text:     "ola",
	})

	if mock.createCount != 1 {
		t.Fatalf("createCount = %d, want 1", mock.createCount)
	}
	if len(mock.createdConvs) != 1 {
		t.Fatalf("created convs = %d, want 1", len(mock.createdConvs))
	}
	// conversation create uses STRING ids.
	if _, ok := mock.createdConvs[0]["contact_id"].(string); !ok {
		t.Fatalf("contact_id not a string: %#v", mock.createdConvs[0]["contact_id"])
	}
	if len(mock.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(mock.messages))
	}
	msg := mock.messages[0]
	if msg["message_type"] != "incoming" {
		t.Fatalf("message_type = %v, want incoming", msg["message_type"])
	}
	if msg["source_id"] != "WAID:ABC123" {
		t.Fatalf("source_id = %v", msg["source_id"])
	}
	if msg["content"] != "ola" {
		t.Fatalf("content = %v", msg["content"])
	}

	// contact create body: phone_number set (jid has '@'), identifier = full jid.
	cr := mock.createReqs[0]
	if cr["phone_number"] != "+5512981201631" {
		t.Fatalf("phone_number = %v", cr["phone_number"])
	}
	if cr["identifier"] != "5512981201631@s.whatsapp.net" {
		t.Fatalf("identifier = %v", cr["identifier"])
	}

	// Second message reuses cached contact+conversation (no new create).
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "5512981201631@s.whatsapp.net", MsgID: "DEF456", Text: "again",
	})
	if mock.createCount != 1 {
		t.Fatalf("second message re-created contact: createCount = %d", mock.createCount)
	}
	if len(mock.createdConvs) != 1 {
		t.Fatalf("second message re-created conversation: %d", len(mock.createdConvs))
	}
	if len(mock.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(mock.messages))
	}
}

func TestChatwootHandleInbound_Outgoing(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
	})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "5512981201631@s.whatsapp.net", FromMe: true, MsgID: "X", Text: "hi",
	})
	if len(mock.messages) != 1 || mock.messages[0]["message_type"] != "outgoing" {
		t.Fatalf("expected one outgoing message, got %#v", mock.messages)
	}
}

func TestChatwootHandleInbound_GroupDropped(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
		IgnoreJids: []string{"@g.us"},
	})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "123456@g.us", MsgID: "G", Text: "group msg",
	})
	if mock.createCount != 0 || len(mock.messages) != 0 {
		t.Fatalf("group message was not dropped: creates=%d msgs=%d", mock.createCount, len(mock.messages))
	}
}

func TestChatwootHandleInbound_StatusBroadcastDropped(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
	})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "status@broadcast", MsgID: "S", Text: "status",
	})
	if len(mock.messages) != 0 {
		t.Fatalf("status@broadcast not dropped")
	}
}

func TestChatwootHandleInbound_Disabled(t *testing.T) {
	mock := newFullMock()
	srv, _ := newInboundServer(t, mock, chatwootConfig{Enabled: false, AccountID: "1", Token: "t", NameInbox: "bot1"})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "5512981201631@s.whatsapp.net", MsgID: "Z", Text: "x",
	})
	if len(mock.messages) != 0 {
		t.Fatalf("disabled config still posted a message")
	}
}

func TestChatwootHandleInbound_Media(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
	})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID:      "5512981201631@s.whatsapp.net",
		MsgID:    "M1",
		Text:     "caption here",
		IsMedia:  true,
		FileName: "photo.jpg",
		Download: func() ([]byte, string, error) { return []byte("JPEGDATA"), "image/jpeg", nil },
	})
	if len(mock.mediaMessages) != 1 {
		t.Fatalf("media messages = %d, want 1", len(mock.mediaMessages))
	}
	rec := mock.mediaMessages[0]
	if rec.Content != "caption here" || rec.MessageType != "incoming" || rec.SourceID != "WAID:M1" {
		t.Fatalf("media fields wrong: %#v", rec)
	}
	if rec.FileName != "photo.jpg" || string(rec.Data) != "JPEGDATA" {
		t.Fatalf("media attachment wrong: %q / %q", rec.FileName, rec.Data)
	}
}

func TestChatwootHandleInbound_ReuseExistingConversation(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	// Pre-existing open conversation in the inbox for a known contact.
	mock.contacts = []cwContact{{ID: 201, Identifier: "5512981201631@s.whatsapp.net", PhoneNumber: "+5512981201631"}}
	mock.conversations = []cwConversation{{ID: 777, InboxID: 50, Status: "open"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1", MergeBrazilContacts: true,
	})
	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "5512981201631@s.whatsapp.net", MsgID: "R1", Text: "hello",
	})
	if mock.createCount != 0 {
		t.Fatalf("existing contact was re-created")
	}
	if len(mock.createdConvs) != 0 {
		t.Fatalf("reused conversation but a new one was created")
	}
	if len(mock.messages) != 1 {
		t.Fatalf("message not posted to reused conversation")
	}
}

func TestChatwootHandleInbound_QuotedReplyLinkage(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv, _ := newInboundServer(t, mock, chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
		MergeBrazilContacts: true, IgnoreJids: []string{"@g.us"},
	})
	ctx := context.Background()
	jid := "5512981201631@s.whatsapp.net"

	// First message: bridged, records WAID(WA1) -> chatwoot id 900 (mock.nextMsg).
	srv.chatwootHandleInbound(ctx, "bot1", InboundMessage{
		JID: jid, MsgID: "WA1", Text: "first",
	})
	if len(mock.messages) != 1 {
		t.Fatalf("first message not created: %d", len(mock.messages))
	}
	firstCwID, ok := srv.chatwootMsgs.chatwootIDForWA("bot1", "WA1")
	if !ok || firstCwID == 0 {
		t.Fatalf("first message not recorded in msg store: id=%d ok=%v", firstCwID, ok)
	}

	// Second message quotes the first -> create payload must carry in_reply_to.
	srv.chatwootHandleInbound(ctx, "bot1", InboundMessage{
		JID: jid, MsgID: "WA2", Text: "reply to first", QuotedWAID: "WA1",
	})
	if len(mock.messages) != 2 {
		t.Fatalf("second message not created: %d", len(mock.messages))
	}
	ca, ok := mock.messages[1]["content_attributes"].(map[string]any)
	if !ok {
		t.Fatalf("content_attributes missing/wrong type: %#v", mock.messages[1])
	}
	// JSON numbers decode to float64.
	if int(ca["in_reply_to"].(float64)) != firstCwID {
		t.Fatalf("in_reply_to = %v, want %d", ca["in_reply_to"], firstCwID)
	}
}
