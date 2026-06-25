package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// mockChatwoot is a tiny stand-in for a Chatwoot install: it serves inbox_list
// and inboxes (create), recording what it received.
type mockChatwoot struct {
	mu        sync.Mutex
	inboxes   []cwInbox
	nextID    int
	lastToken string
	createdWH string
}

func newMockChatwoot() *mockChatwoot { return &mockChatwoot{nextID: 100} }

func (m *mockChatwoot) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/accounts/{acct}/inbox_list", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.lastToken = r.Header.Get("api_access_token")
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": m.inboxes})
	})
	mux.HandleFunc("POST /api/v1/accounts/{acct}/inboxes", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name    string `json:"name"`
			Channel struct {
				Type       string `json:"type"`
				WebhookURL string `json:"webhook_url"`
			} `json:"channel"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.createdWH = body.Channel.WebhookURL
		in := cwInbox{ID: m.nextID, Name: body.Name}
		m.nextID++
		m.inboxes = append(m.inboxes, in)
		_ = json.NewEncoder(w).Encode(in)
	})
	return mux
}

func TestChatwootEnsureInbox(t *testing.T) {
	mock := newMockChatwoot()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cw := newChatwootClient(chatwootConfig{URL: srv.URL, Token: "tok123", AccountID: "1"})
	// Absent -> created.
	in, err := cw.EnsureInbox(context.Background(), "5512988624928", "https://wa/chatwoot/webhook/inst")
	if err != nil {
		t.Fatalf("EnsureInbox create: %v", err)
	}
	if in.ID == 0 || in.Name != "5512988624928" {
		t.Fatalf("created inbox = %+v", in)
	}
	if mock.lastToken != "tok123" {
		t.Fatalf("api_access_token = %q", mock.lastToken)
	}
	if mock.createdWH != "https://wa/chatwoot/webhook/inst" {
		t.Fatalf("webhook_url = %q", mock.createdWH)
	}
	// Present -> reused (no new id).
	in2, err := cw.EnsureInbox(context.Background(), "5512988624928", "x")
	if err != nil {
		t.Fatalf("EnsureInbox reuse: %v", err)
	}
	if in2.ID != in.ID {
		t.Fatalf("reuse id = %d, want %d", in2.ID, in.ID)
	}
}

func TestChatwootSetAndFind(t *testing.T) {
	mock := newMockChatwoot()
	cwsrv := httptest.NewServer(mock.handler())
	defer cwsrv.Close()

	fb := newFakeBackend()
	_ = fb.Create("bot1")
	srv := New(Options{APIKey: testKey, Backend: fb, Logger: log.New(io.Discard, "", 0)})
	h := srv.Handler()

	// set with autoCreate -> persists config + creates the inbox in Chatwoot.
	rec := do(t, h, "POST", "/chatwoot/set/bot1", testKey, chatwootConfig{
		Enabled: true, URL: cwsrv.URL, AccountID: "1", Token: "tok", AutoCreate: true,
		MergeBrazilContacts: true, IgnoreJids: []string{"@g.us"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("set status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var setResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &setResp)
	if setResp["webhook_url"] == "" {
		t.Fatalf("no webhook_url in set response")
	}
	if setResp["nameInbox"] != "bot1" { // defaulted to instance name
		t.Fatalf("nameInbox = %v, want bot1", setResp["nameInbox"])
	}
	if len(mock.inboxes) != 1 || mock.inboxes[0].Name != "bot1" {
		t.Fatalf("inbox not created in chatwoot: %+v", mock.inboxes)
	}

	// find returns the persisted config.
	rec = do(t, h, "GET", "/chatwoot/find/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("find status = %d", rec.Code)
	}
	var found map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &found)
	if found["enabled"] != true || found["accountId"] != "1" {
		t.Fatalf("find = %+v", found)
	}
}

func TestChatwootWebhook_NoAuth(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	srv := New(Options{APIKey: testKey, Backend: fb, Logger: log.New(io.Discard, "", 0)})
	h := srv.Handler()
	// webhook is reachable WITHOUT the apikey (Chatwoot posts here).
	rec := do(t, h, "POST", "/chatwoot/webhook/bot1", "", map[string]any{"event": "message_created"})
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200 (no auth)", rec.Code)
	}
}
