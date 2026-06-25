package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// chatwoot.go — Phase 1 of the Evolution-compatible Chatwoot integration:
// per-instance config (persisted), the Chatwoot Application API client, and the
// /chatwoot/set | /find | /webhook routes. Inbound (WhatsApp->Chatwoot) and
// outbound (Chatwoot->WhatsApp) bridging come in later phases.

// chatwootConfig mirrors Evolution's ChatwootDto (the per-instance "provider").
type chatwootConfig struct {
	Enabled             bool     `json:"enabled"`
	AccountID           string   `json:"accountId"`
	Token               string   `json:"token"`
	URL                 string   `json:"url"`
	NameInbox           string   `json:"nameInbox"`
	SignMsg             bool     `json:"signMsg"`
	SignDelimiter       string   `json:"signDelimiter,omitempty"`
	Number              string   `json:"number,omitempty"`
	ReopenConversation  bool     `json:"reopenConversation"`
	ConversationPending bool     `json:"conversationPending"`
	MergeBrazilContacts bool     `json:"mergeBrazilContacts"`
	ImportContacts      bool     `json:"importContacts"`
	ImportMessages      bool     `json:"importMessages"`
	AutoCreate          bool     `json:"autoCreate"`
	Organization        string   `json:"organization,omitempty"`
	Logo                string   `json:"logo,omitempty"`
	IgnoreJids          []string `json:"ignoreJids,omitempty"`
}

// --- per-instance config store (persisted to <instance>.chatwoot JSON) ---

type chatwootStore struct {
	mu  sync.Mutex
	dir string
	cfg map[string]chatwootConfig
}

func newChatwootStore() *chatwootStore {
	return &chatwootStore{cfg: map[string]chatwootConfig{}}
}

// setDir points the store at a persistence directory and loads existing sidecars.
func (s *chatwootStore) setDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dir = dir
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".chatwoot") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".chatwoot")
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c chatwootConfig
		if json.Unmarshal(b, &c) == nil {
			s.cfg[name] = c
		}
	}
}

func (s *chatwootStore) get(instance string) (chatwootConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cfg[instance]
	return c, ok
}

func (s *chatwootStore) set(instance string, c chatwootConfig) {
	s.mu.Lock()
	s.cfg[instance] = c
	dir := s.dir
	s.mu.Unlock()
	if dir != "" {
		if b, err := json.Marshal(c); err == nil {
			_ = os.WriteFile(filepath.Join(dir, instance+".chatwoot"), b, 0o600)
		}
	}
}

// --- Chatwoot Application API client ---

// chatwootClient talks to a Chatwoot install for one instance's provider config.
// Auth is the api_access_token header (a user/agent token, NOT Bearer). All paths
// are /api/v1/accounts/{accountId}/...
type chatwootClient struct {
	base                string // provider.url, no trailing slash
	token               string
	accountID           string
	mergeBrazilContacts bool
	hc                  *http.Client
}

func newChatwootClient(c chatwootConfig) *chatwootClient {
	return &chatwootClient{
		base:                strings.TrimRight(c.URL, "/"),
		token:               c.Token,
		accountID:           c.AccountID,
		mergeBrazilContacts: c.MergeBrazilContacts,
		hc:                  &http.Client{Timeout: 30 * time.Second},
	}
}

func (cw *chatwootClient) acctPath(p string) string {
	return fmt.Sprintf("%s/api/v1/accounts/%s%s", cw.base, cw.accountID, p)
}

// do issues a JSON request and decodes the JSON response into out (out may be nil).
func (cw *chatwootClient) do(ctx context.Context, method, url string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("api_access_token", cw.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cw.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("chatwoot %s %s: %d %s", method, url, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// cwInbox is the subset of a Chatwoot inbox we need.
type cwInbox struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ListInboxes returns the account's inboxes (GET inbox_list -> {payload:[...]}).
func (cw *chatwootClient) ListInboxes(ctx context.Context) ([]cwInbox, error) {
	var resp struct {
		Payload []cwInbox `json:"payload"`
	}
	if err := cw.do(ctx, http.MethodGet, cw.acctPath("/inbox_list"), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Payload, nil
}

// CreateInbox creates an "api"-channel inbox whose webhook_url points back at us.
func (cw *chatwootClient) CreateInbox(ctx context.Context, name, webhookURL string) (cwInbox, error) {
	body := map[string]any{
		"name": name,
		"channel": map[string]any{
			"type":        "api",
			"webhook_url": webhookURL,
		},
	}
	var inbox cwInbox
	err := cw.do(ctx, http.MethodPost, cw.acctPath("/inboxes"), body, &inbox)
	return inbox, err
}

// EnsureInbox returns the inbox named name, creating it (api channel) if absent.
func (cw *chatwootClient) EnsureInbox(ctx context.Context, name, webhookURL string) (cwInbox, error) {
	inboxes, err := cw.ListInboxes(ctx)
	if err != nil {
		return cwInbox{}, err
	}
	for _, in := range inboxes {
		if in.Name == name {
			return in, nil
		}
	}
	return cw.CreateInbox(ctx, name, webhookURL)
}

// --- handlers ---

// chatwootWebhookURL builds the URL Chatwoot posts agent replies to.
func (s *Server) chatwootWebhookURL(r *http.Request, instance string) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	host := r.Host
	return fmt.Sprintf("%s://%s/chatwoot/webhook/%s", scheme, host, instance)
}

// handleChatwootSet: POST /chatwoot/set/{instance}. Persists the config and, when
// autoCreate, ensures the Chatwoot inbox exists with our webhook URL.
func (s *Server) handleChatwootSet(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")
	if !s.backend.Exists(instance) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var cfg chatwootConfig
	if !s.decodeJSON(w, r, &cfg) {
		return
	}
	if cfg.Enabled {
		if cfg.URL == "" || cfg.AccountID == "" || cfg.Token == "" {
			s.writeError(w, http.StatusBadRequest, "url, accountId and token are required when enabled")
			return
		}
	}
	if cfg.NameInbox == "" {
		cfg.NameInbox = strings.SplitN(instance, "-cwId-", 2)[0]
	}
	if !cfg.SignMsg {
		cfg.SignDelimiter = ""
	}
	s.chatwoot.set(instance, cfg)

	webhookURL := s.chatwootWebhookURL(r, instance)
	if cfg.Enabled && cfg.AutoCreate {
		cw := newChatwootClient(cfg)
		if _, err := cw.EnsureInbox(r.Context(), cfg.NameInbox, webhookURL); err != nil {
			s.logger.Printf("chatwoot: ensure inbox for %q: %v", instance, err)
			s.writeError(w, http.StatusBadGateway, "chatwoot inbox setup failed: "+err.Error())
			return
		}
	}
	resp := map[string]any{}
	b, _ := json.Marshal(cfg)
	_ = json.Unmarshal(b, &resp)
	resp["webhook_url"] = webhookURL
	s.writeJSON(w, http.StatusCreated, resp)
}

// handleChatwootFind: GET /chatwoot/find/{instance}.
func (s *Server) handleChatwootFind(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")
	cfg, ok := s.chatwoot.get(instance)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false, "url": "", "accountId": "", "token": "",
			"signMsg": false, "nameInbox": "", "webhook_url": "",
		})
		return
	}
	resp := map[string]any{}
	b, _ := json.Marshal(cfg)
	_ = json.Unmarshal(b, &resp)
	resp["webhook_url"] = s.chatwootWebhookURL(r, instance)
	s.writeJSON(w, http.StatusOK, resp)
}

// handleChatwootWebhook (POST /chatwoot/webhook/{instance}, NO apikey) lives in
// chatwoot_outbound.go (Phase 3): it forwards Chatwoot agent replies to WhatsApp.
