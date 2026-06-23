package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	wa "github.com/felipeleal/wa-go/wa"
)

// --- fakeBackend ---
//
// Mirrors the manager's fake-session approach: it lets every HTTP route be
// exercised offline (no Manager, no Noise handshake, no network). Each method
// records its inputs and returns canned outputs the tests assert on.

type sentText struct{ name, jid, text string }
type sentMedia struct {
	name, jid string
	media     MediaArg
}

type fakeBackend struct {
	mu sync.Mutex

	exists   map[string]bool
	status   map[string]string
	qr       string
	connErr  error
	texts    []sentText
	medias   []sentMedia
	messages map[string][]StoredMsg // jid -> stored
	numbers  []NumberStatus
	groups   []GroupArg
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		exists:   map[string]bool{},
		status:   map[string]string{},
		messages: map[string][]StoredMsg{},
	}
}

func (f *fakeBackend) Create(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exists[name] = true
	f.status[name] = "connecting"
	return nil
}

func (f *fakeBackend) Connect(ctx context.Context, name string) (string, error) {
	if f.connErr != nil {
		return "", f.connErr
	}
	return f.qr, nil
}

func (f *fakeBackend) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.exists[name] {
		return ErrInstanceNotFound
	}
	delete(f.exists, name)
	delete(f.status, name)
	return nil
}

func (f *fakeBackend) Logout(name string) error {
	if !f.Exists(name) {
		return ErrInstanceNotFound
	}
	return nil
}

func (f *fakeBackend) Status() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for k, v := range f.status {
		out[k] = v
	}
	return out
}

func (f *fakeBackend) Exists(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exists[name]
}

func (f *fakeBackend) SendText(ctx context.Context, name, jid, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, sentText{name, jid, text})
	return "MSGID-TEXT", nil
}

func (f *fakeBackend) SendMedia(ctx context.Context, name, jid string, m MediaArg) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.medias = append(f.medias, sentMedia{name, jid, m})
	return "MSGID-MEDIA", nil
}

func (f *fakeBackend) SendReaction(ctx context.Context, name, jid, msgID string, fromMe bool, emoji string) (string, error) {
	return "MSGID-REACT", nil
}

func (f *fakeBackend) FindMessages(name, jid string, limit int) ([]StoredMsg, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messages[jid], nil
}

func (f *fakeBackend) WhatsAppNumbers(ctx context.Context, name string, numbers []string) ([]NumberStatus, error) {
	return f.numbers, nil
}

func (f *fakeBackend) Groups(ctx context.Context, name string) ([]GroupArg, error) {
	return f.groups, nil
}

func (f *fakeBackend) GroupMetadata(ctx context.Context, name, jid string) (GroupArg, error) {
	if len(f.groups) == 0 {
		return GroupArg{}, ErrInstanceNotFound
	}
	return f.groups[0], nil
}

// --- helpers ---

const testKey = "secret-key"

func newTestServer(t *testing.T, fb *fakeBackend) http.Handler {
	t.Helper()
	srv := New(Options{
		APIKey:  testKey,
		Backend: fb,
		Logger:  log.New(io.Discard, "", 0),
	})
	return srv.Handler()
}

func do(t *testing.T, h http.Handler, method, path, apikey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if apikey != "" {
		req.Header.Set("apikey", apikey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- auth ---

func TestAuth_MissingKey401(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/instance/fetchInstances", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_WrongKey401(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/instance/fetchInstances", "nope", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_CorrectKeyOK(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/instance/fetchInstances", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// --- create + fetchInstances ---

func TestCreateAndFetchInstances(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)

	rec := do(t, h, "POST", "/instance/create", testKey, createInstanceReq{InstanceName: "bot1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var cr createInstanceResp
	if err := json.Unmarshal(rec.Body.Bytes(), &cr); err != nil {
		t.Fatalf("decode create resp: %v", err)
	}
	if cr.Instance.InstanceName != "bot1" {
		t.Fatalf("instanceName = %q, want bot1", cr.Instance.InstanceName)
	}
	if cr.Instance.ConnectionStatus != "connecting" {
		t.Fatalf("connectionStatus = %q, want connecting", cr.Instance.ConnectionStatus)
	}

	rec = do(t, h, "GET", "/instance/fetchInstances", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("fetch status = %d", rec.Code)
	}
	var list []instanceInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].InstanceName != "bot1" {
		t.Fatalf("fetchInstances = %+v, want [bot1]", list)
	}
}

func TestCreate_RequiresName(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/instance/create", testKey, createInstanceReq{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- connect (QR PNG base64) ---

func TestConnect_ReturnsQRBase64(t *testing.T) {
	fb := newFakeBackend()
	fb.qr = "2@abc,def,ghi"
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)

	rec := do(t, h, "GET", "/instance/connect/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var cr connectResp
	if err := json.Unmarshal(rec.Body.Bytes(), &cr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// code is the base64 PNG; it must decode to a non-empty PNG.
	raw, err := base64.StdEncoding.DecodeString(cr.Code)
	if err != nil {
		t.Fatalf("code is not base64: %v", err)
	}
	if len(raw) < 8 || string(raw[1:4]) != "PNG" {
		t.Fatalf("code is not a PNG (len=%d)", len(raw))
	}
	if cr.Base64 == "" || cr.Base64[:11] != "data:image/" {
		t.Fatalf("base64 data URI missing: %q", cr.Base64)
	}
}

func TestConnect_NotFound404(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/instance/connect/ghost", testKey, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- sendText ---

func TestSendText(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)

	rec := do(t, h, "POST", "/message/sendText/bot1", testKey, sendTextReq{Number: "5512999", Text: "hi there"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var sr sendResp
	if err := json.Unmarshal(rec.Body.Bytes(), &sr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sr.Key.ID != "MSGID-TEXT" {
		t.Fatalf("key.id = %q, want MSGID-TEXT", sr.Key.ID)
	}
	if sr.Status != sendStatusPending {
		t.Fatalf("status = %q, want %q", sr.Status, sendStatusPending)
	}
	if sr.Key.RemoteJID != "5512999@s.whatsapp.net" {
		t.Fatalf("remoteJid = %q, want normalized", sr.Key.RemoteJID)
	}
	// The backend recorded the normalized JID.
	if len(fb.texts) != 1 || fb.texts[0].jid != "5512999@s.whatsapp.net" || fb.texts[0].text != "hi there" {
		t.Fatalf("backend recorded %+v", fb.texts)
	}
}

func TestSendText_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendText/bot1", testKey, sendTextReq{Number: "5512"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- sendMedia ---

func TestSendMedia(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)

	payload := []byte("fake-image-bytes")
	b64 := base64.StdEncoding.EncodeToString(payload)
	rec := do(t, h, "POST", "/message/sendMedia/bot1", testKey, sendMediaReq{
		Number:    "5512999",
		MediaType: "image",
		Media:     "data:image/png;base64," + b64,
		Caption:   "look",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var sr sendResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if sr.Key.ID != "MSGID-MEDIA" {
		t.Fatalf("key.id = %q", sr.Key.ID)
	}
	if len(fb.medias) != 1 {
		t.Fatalf("media not recorded")
	}
	m := fb.medias[0]
	if m.media.Kind != "image" || string(m.media.Data) != "fake-image-bytes" || m.media.Caption != "look" {
		t.Fatalf("media decoded wrong: %+v data=%q", m.media, m.media.Data)
	}
}

func TestSendMedia_BadType(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendMedia/bot1", testKey, sendMediaReq{
		Number: "5512", MediaType: "gif", Media: "AAAA",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- sendReaction ---

func TestSendReaction(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendReaction/bot1", testKey, sendReactionReq{
		Key:      messageKey{RemoteJID: "5512999", ID: "ABC", FromMe: false},
		Reaction: "👍",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var sr sendResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if sr.Key.ID != "MSGID-REACT" {
		t.Fatalf("key.id = %q", sr.Key.ID)
	}
}

// --- findMessages ---

func TestFindMessages(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.messages["5512999@s.whatsapp.net"] = []StoredMsg{
		{ID: "M1", ChatJID: "5512999@s.whatsapp.net", Timestamp: 100, Text: "hello", Type: "text"},
		{ID: "M2", ChatJID: "5512999@s.whatsapp.net", Timestamp: 200, Text: "world", Type: "text"},
	}
	h := newTestServer(t, fb)

	body := findMessagesReq{Limit: 10}
	body.Where.Key.RemoteJID = "5512999"
	rec := do(t, h, "POST", "/chat/findMessages/bot1", testKey, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var fr findMessagesResp
	if err := json.Unmarshal(rec.Body.Bytes(), &fr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fr.Messages.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(fr.Messages.Records))
	}
	if fr.Messages.Records[0].Key.ID != "M1" || fr.Messages.Records[0].Message.Conversation != "hello" {
		t.Fatalf("record[0] = %+v", fr.Messages.Records[0])
	}
}

func TestFindMessages_EmptyDocumented(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	body := findMessagesReq{}
	body.Where.Key.RemoteJID = "5599"
	rec := do(t, h, "POST", "/chat/findMessages/bot1", testKey, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var fr findMessagesResp
	_ = json.Unmarshal(rec.Body.Bytes(), &fr)
	if len(fr.Messages.Records) != 0 {
		t.Fatalf("want empty records, got %d", len(fr.Messages.Records))
	}
	// The field must serialize as an array, not null.
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"records":[]`)) {
		t.Fatalf("records not empty array: %s", rec.Body.String())
	}
}

// --- whatsappNumbers ---

func TestWhatsAppNumbers(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.numbers = []NumberStatus{
		{Number: "5512999", JID: "5512999@s.whatsapp.net", Exists: true},
		{Number: "5500000", JID: "", Exists: false},
	}
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/whatsappNumbers/bot1", testKey, whatsappNumbersReq{Numbers: []string{"5512999", "5500000"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []numberStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 || !out[0].Exists || out[1].Exists {
		t.Fatalf("out = %+v", out)
	}
}

// --- groups ---

func TestFetchAllGroups(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.groups = []GroupArg{
		{JID: "123@g.us", Subject: "Fam", Participants: []GroupParticipantArg{{JID: "a@s.whatsapp.net", Admin: "admin"}}},
	}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/group/fetchAllGroups/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out []groupRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Subject != "Fam" || len(out[0].Participants) != 1 {
		t.Fatalf("groups = %+v", out)
	}
}

// --- delete + logout ---

func TestDeleteAndLogout(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)

	rec := do(t, h, "GET", "/instance/logout/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
	rec = do(t, h, "DELETE", "/instance/delete/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if fb.Exists("bot1") {
		t.Fatalf("instance not deleted")
	}
	rec = do(t, h, "DELETE", "/instance/delete/bot1", testKey, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("re-delete status = %d, want 404", rec.Code)
	}
}

// --- webhook set ---

func TestSetWebhook(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	srv := New(Options{APIKey: testKey, Backend: fb, Logger: log.New(io.Discard, "", 0)})
	h := srv.Handler()

	rec := do(t, h, "POST", "/webhook/set/bot1", testKey, setWebhookReq{URL: "http://example/hook"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := srv.Dispatcher().url("bot1"); got != "http://example/hook" {
		t.Fatalf("dispatcher url = %q", got)
	}
}

// --- webhook dispatch (event -> Evolution POST) ---

func TestWebhookDispatch_MessageUpsert(t *testing.T) {
	// Destination captures the POSTed envelope.
	var (
		mu      sync.Mutex
		gotBody []byte
		gotCT   string
	)
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = b
		gotCT = r.Header.Get("Content-Type")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	d := newWebhookDispatcher(dest.Client(), log.New(io.Discard, "", 0))
	d.set("bot1", dest.URL)

	ev := wa.MessageEvent{
		From:      "5512999@s.whatsapp.net",
		ID:        "WAMID1",
		Timestamp: 1718900000,
		PushName:  "Felipe",
		Type:      wa.MessageType("text"),
		Text:      "olá",
	}
	d.dispatch(context.Background(), "bot1", ev)

	mu.Lock()
	body := append([]byte(nil), gotBody...)
	ct := gotCT
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("destination received no body")
	}
	if ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var env webhookEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, body)
	}
	if env.Event != "messages.upsert" {
		t.Fatalf("event = %q, want messages.upsert", env.Event)
	}
	if env.Instance != "bot1" {
		t.Fatalf("instance = %q", env.Instance)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data not an object: %T", env.Data)
	}
	key, _ := data["key"].(map[string]interface{})
	if key["id"] != "WAMID1" || key["remoteJid"] != "5512999@s.whatsapp.net" {
		t.Fatalf("data.key = %+v", key)
	}
	msg, _ := data["message"].(map[string]interface{})
	if msg["conversation"] != "olá" {
		t.Fatalf("data.message.conversation = %v", msg["conversation"])
	}
	if data["pushName"] != "Felipe" {
		t.Fatalf("data.pushName = %v", data["pushName"])
	}
}

func TestWebhookDispatch_NoURLNoPost(t *testing.T) {
	var called int32
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = 1
		w.WriteHeader(200)
	}))
	defer dest.Close()
	d := newWebhookDispatcher(dest.Client(), log.New(io.Discard, "", 0))
	// No URL set for "bot1".
	d.dispatch(context.Background(), "bot1", wa.MessageEvent{ID: "X"})
	time.Sleep(20 * time.Millisecond)
	if called != 0 {
		t.Fatal("dispatch POSTed despite no configured webhook URL")
	}
}

func TestWebhookDispatch_ConnectionUpdate(t *testing.T) {
	var got webhookEnvelope
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
	}))
	defer dest.Close()
	d := newWebhookDispatcher(dest.Client(), log.New(io.Discard, "", 0))
	d.set("bot1", dest.URL)
	d.dispatch(context.Background(), "bot1", wa.DisconnectedEvent{Reason: "stream-end"})

	if got.Event != "connection.update" {
		t.Fatalf("event = %q, want connection.update", got.Event)
	}
	data, _ := got.Data.(map[string]interface{})
	if data["state"] != "close" || data["statusReason"] != "stream-end" {
		t.Fatalf("data = %+v", data)
	}
}

// --- RunEventPump integration (dispatch + ChatStore feed) ---

func TestRunEventPump_FeedsChatStoreAndDispatches(t *testing.T) {
	var gotEvent string
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env webhookEnvelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		gotEvent = env.Event
		w.WriteHeader(200)
	}))
	defer dest.Close()

	d := newWebhookDispatcher(dest.Client(), log.New(io.Discard, "", 0))
	d.set("bot1", dest.URL)

	cs := wa.NewChatStore()
	feed := feedChatStore(func(instance string) *wa.ChatStore {
		if instance == "bot1" {
			return cs
		}
		return nil
	})

	// Drive the pump directly with a synthetic event by invoking dispatch+feed as
	// RunEventPump would (RunEventPump itself needs a *wa.Manager; the unit of
	// logic — feed then dispatch — is what we assert here).
	ev := wa.MessageEvent{
		From: "5512@s.whatsapp.net", ID: "WAMID9", Type: wa.MessageType("text"), Text: "hey", Timestamp: 10,
	}
	feed("bot1", ev)
	d.dispatch(context.Background(), "bot1", ev)

	if gotEvent != "messages.upsert" {
		t.Fatalf("dispatched event = %q", gotEvent)
	}
	msgs := cs.ChatMessages("5512@s.whatsapp.net", 10)
	if len(msgs) != 1 || msgs[0].Text != "hey" {
		t.Fatalf("chatstore messages = %+v", msgs)
	}
}
