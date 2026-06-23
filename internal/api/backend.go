package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wa "github.com/felipeleal/wa-go/wa"
)

// Backend is the seam between the HTTP handlers and the WhatsApp session stack.
// Handlers depend only on this interface, so the whole API surface is testable
// offline with a fake (no Manager, no Noise handshake, no network). The
// production implementation (ManagerBackend) adapts *wa.Manager.
type Backend interface {
	// Create registers a new instance (opens its store, adds it to the manager).
	Create(name string) error
	// Connect starts the instance and returns the latest QR code string captured
	// from its event stream (empty if already logged in / none yet).
	Connect(ctx context.Context, name string) (qr string, err error)
	// Delete removes an instance (stops + closes its store).
	Delete(name string) error
	// Logout disconnects an instance without deleting its store.
	Logout(name string) error
	// Status returns name -> Evolution connectionStatus (open|connecting|close).
	Status() map[string]string
	// Exists reports whether the named instance is registered.
	Exists(name string) bool

	// SendText sends a text message; returns the message id.
	SendText(ctx context.Context, name, jid, text string) (string, error)
	// SendMedia sends an image/video/audio/document; returns the message id.
	SendMedia(ctx context.Context, name, jid string, m MediaArg) (string, error)
	// SendReaction reacts to a target message; returns the reaction message id.
	SendReaction(ctx context.Context, name, jid, msgID string, fromMe bool, emoji string) (string, error)

	// FindMessages returns stored messages for a chat (newest last), bounded by
	// limit (0 = all available).
	FindMessages(name, jid string, limit int) ([]StoredMsg, error)
	// WhatsAppNumbers resolves which numbers are on WhatsApp.
	WhatsAppNumbers(ctx context.Context, name string, numbers []string) ([]NumberStatus, error)
	// Groups returns metadata for all groups the instance participates in.
	Groups(ctx context.Context, name string) ([]GroupArg, error)
	// GroupMetadata returns metadata for one group.
	GroupMetadata(ctx context.Context, name, jid string) (GroupArg, error)

	// SendPresence sets typing/availability. For composing|paused it targets the
	// given chat jid (per-chat typing); for available|unavailable it is a global
	// presence and jid is ignored.
	SendPresence(ctx context.Context, name, jid, presence string) error
	// MarkRead sends read receipts for the given message ids on a chat.
	MarkRead(ctx context.Context, name, jid string, ids []string) error

	// GroupCreate creates a new group and returns its metadata.
	GroupCreate(ctx context.Context, name, subject string, participants []string) (GroupArg, error)
	// GroupUpdateParticipants adds/removes/promotes/demotes members; returns the
	// per-participant result.
	GroupUpdateParticipants(ctx context.Context, name, groupJID, action string, participants []string) ([]ParticipantResult, error)
	// GroupInviteCode returns the group's current invite code.
	GroupInviteCode(ctx context.Context, name, groupJID string) (string, error)
	// GroupLeave leaves a group.
	GroupLeave(ctx context.Context, name, groupJID string) error
}

// ParticipantResult is the backend-neutral outcome of a group participant update.
type ParticipantResult struct {
	JID    string
	Status string
}

// MediaArg carries decoded media bytes plus metadata for SendMedia.
type MediaArg struct {
	Kind     string // image|video|audio|document
	Data     []byte
	Caption  string
	FileName string
	Mimetype string
}

// StoredMsg is the backend-neutral view of a stored message.
type StoredMsg struct {
	ID        string
	ChatJID   string
	Timestamp int64
	Text      string
	Type      string
	FromMe    bool
}

// NumberStatus is the backend-neutral OnWhatsApp result.
type NumberStatus struct {
	Number string
	JID    string
	Exists bool
}

// GroupArg is the backend-neutral group metadata.
type GroupArg struct {
	JID          string
	Subject      string
	Owner        string
	Desc         string
	Creation     int64
	Participants []GroupParticipantArg
}

// GroupParticipantArg is one group member.
type GroupParticipantArg struct {
	JID   string
	Admin string
}

// ErrInstanceNotFound is returned when an operation names an unknown instance.
var ErrInstanceNotFound = errors.New("instance not found")

// ErrNoSession is returned when an instance exists but has no live session yet
// (not logged in), so a send/query cannot proceed.
var ErrNoSession = errors.New("instance has no active session")

// --- production backend backed by the Manager ---

// ManagerBackend adapts *wa.Manager + per-instance stores and an in-memory
// ChatStore (fed by the webhook event pump) into the Backend interface.
type ManagerBackend struct {
	mgr *wa.Manager
	dir string

	mu        sync.Mutex
	instances map[string]*mbInstance
}

type mbInstance struct {
	name  string
	store wa.Store
	mc    *wa.ManagedClient
	chats *wa.ChatStore
	qr    string // latest QR code captured from the event stream
}

// NewManagerBackend builds the production Backend backed by the given Manager,
// writing per-instance SQLite stores under dir. The returned concrete type also
// exposes ChatStore and SetQR so the host's event pump can feed message history
// and capture QR codes (see cmd/wa-server).
func NewManagerBackend(mgr *wa.Manager, dir string) *ManagerBackend {
	return &ManagerBackend{
		mgr:       mgr,
		dir:       dir,
		instances: make(map[string]*mbInstance),
	}
}

// ChatStore returns the per-instance ChatStore (for the event pump's feed). It
// returns nil for unknown instances.
func (b *ManagerBackend) ChatStore(name string) *wa.ChatStore { return b.chatStore(name) }

// SetQR records the latest QR code for an instance (called by the event pump).
func (b *ManagerBackend) SetQR(name, code string) { b.setQR(name, code) }

func (b *ManagerBackend) get(name string) (*mbInstance, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	in, ok := b.instances[name]
	return in, ok
}

func (b *ManagerBackend) Exists(name string) bool {
	_, ok := b.get(name)
	return ok
}

// chatStore returns the ChatStore for an instance (used by the webhook pump to
// feed incoming messages so findMessages has data).
func (b *ManagerBackend) chatStore(name string) *wa.ChatStore {
	if in, ok := b.get(name); ok {
		return in.chats
	}
	return nil
}

// setQR records the latest QR for an instance (called by the event pump).
func (b *ManagerBackend) setQR(name, code string) {
	if in, ok := b.get(name); ok {
		b.mu.Lock()
		in.qr = code
		b.mu.Unlock()
	}
}

func (b *ManagerBackend) Create(name string) error {
	if name == "" {
		return errors.New("empty instance name")
	}
	b.mu.Lock()
	if _, ok := b.instances[name]; ok {
		b.mu.Unlock()
		return fmt.Errorf("instance %q already exists", name)
	}
	b.mu.Unlock()

	path := filepath.Join(b.dir, name+".db")
	st, err := wa.OpenStore(path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	mc, err := b.mgr.Add(name, st)
	if err != nil {
		_ = st.Close()
		return err
	}
	b.mu.Lock()
	b.instances[name] = &mbInstance{name: name, store: st, mc: mc, chats: wa.NewChatStore()}
	b.mu.Unlock()
	return nil
}

func (b *ManagerBackend) Connect(ctx context.Context, name string) (string, error) {
	in, ok := b.get(name)
	if !ok {
		return "", ErrInstanceNotFound
	}
	// The manager auto-starts instances on Add when already Started, so the QR is
	// produced asynchronously and captured by the event pump into in.qr. Poll
	// briefly for it (bounded) so the HTTP response carries the first QR.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		qr := in.qr
		b.mu.Unlock()
		if qr != "" {
			return qr, nil
		}
		st := b.mgr.Status()[name]
		if st == wa.StateLoggedIn {
			return "", nil // already authenticated, no QR needed
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	b.mu.Lock()
	qr := in.qr
	b.mu.Unlock()
	return qr, nil
}

func (b *ManagerBackend) Delete(name string) error {
	b.mu.Lock()
	in, ok := b.instances[name]
	if ok {
		delete(b.instances, name)
	}
	b.mu.Unlock()
	if !ok {
		return ErrInstanceNotFound
	}
	return in.store.Close()
}

func (b *ManagerBackend) Logout(name string) error {
	if !b.Exists(name) {
		return ErrInstanceNotFound
	}
	// The Manager has no per-instance stop; logout is best-effort a no-op beyond
	// reporting success (full teardown is Delete). Documented limitation.
	return nil
}

func (b *ManagerBackend) Status() map[string]string {
	raw := b.mgr.Status()
	out := make(map[string]string, len(raw))
	for name, st := range raw {
		out[name] = connectionStatus(st)
	}
	// Include registered-but-not-yet-started instances as close.
	b.mu.Lock()
	for name := range b.instances {
		if _, ok := out[name]; !ok {
			out[name] = "close"
		}
	}
	b.mu.Unlock()
	return out
}

// SendText sends a text message through the ManagedClient's live-session handle
// (the lightweight path that does not need the full *wa.Client). The richer
// media/reaction/usync/group methods fetch the live *wa.Client via liveClient.
func (b *ManagerBackend) SendText(ctx context.Context, name, jid, text string) (string, error) {
	in, ok := b.get(name)
	if !ok {
		return "", ErrInstanceNotFound
	}
	return in.mc.SendText(ctx, jid, text)
}

// liveClient returns the instance's current live *wa.Client (the full client API
// beyond SendText), fetched fresh from the ManagedClient per call since the
// Manager rebuilds the client on every reconnection. It returns ErrInstanceNotFound
// for an unknown instance and ErrNoSession when the instance is registered but
// has no live session yet (offline / not logged in).
func (b *ManagerBackend) liveClient(name string) (*wa.Client, error) {
	in, ok := b.get(name)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	c, ok := in.mc.Client()
	if !ok {
		return nil, ErrNoSession
	}
	return c, nil
}

// SendMedia uploads and sends an image/video/audio/document through the live
// client. Media transfer is enabled lazily on the fetched client (the
// EnableDefaultMediaTransfer call is idempotent — it just installs the live
// http.DefaultClient uploader), so the first send on a fresh connection wires the
// upload path before use.
func (b *ManagerBackend) SendMedia(ctx context.Context, name, jid string, m MediaArg) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	c.EnableDefaultMediaTransfer()
	switch m.Kind {
	case "image":
		return c.SendImageBytes(ctx, jid, m.Data, m.Caption, m.Mimetype)
	case "video":
		return c.SendVideoBytes(ctx, jid, m.Data, m.Caption, m.Mimetype)
	case "audio":
		return c.SendAudioBytes(ctx, jid, m.Data, m.Mimetype)
	case "document":
		return c.SendDocumentBytes(ctx, jid, m.Data, m.FileName, m.Mimetype)
	default:
		return "", fmt.Errorf("unsupported media kind %q", m.Kind)
	}
}

// SendReaction reacts to a target message through the live client.
func (b *ManagerBackend) SendReaction(ctx context.Context, name, jid, msgID string, fromMe bool, emoji string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.React(ctx, jid, msgID, fromMe, emoji)
}

func (b *ManagerBackend) FindMessages(name, jid string, limit int) ([]StoredMsg, error) {
	in, ok := b.get(name)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	stored := in.chats.ChatMessages(jid, limit)
	out := make([]StoredMsg, 0, len(stored))
	for _, sm := range stored {
		out = append(out, StoredMsg{
			ID:        sm.Key,
			ChatJID:   sm.FromJID,
			Timestamp: sm.Timestamp,
			Text:      sm.Text,
			Type:      sm.Type,
		})
	}
	return out, nil
}

// WhatsAppNumbers resolves which numbers are on WhatsApp via a live usync query.
// It reads only the exported fields of each result (Query/JID/Exists) so this
// module need not name the library's internal result type.
func (b *ManagerBackend) WhatsAppNumbers(ctx context.Context, name string, numbers []string) ([]NumberStatus, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	results, err := c.OnWhatsApp(ctx, numbers)
	if err != nil {
		return nil, err
	}
	out := make([]NumberStatus, 0, len(results))
	for _, r := range results {
		out = append(out, NumberStatus{Number: r.Query, JID: r.JID, Exists: r.Exists})
	}
	return out, nil
}

// Groups returns metadata for all groups the instance participates in. The
// library exposes per-group metadata (GroupMetadata) but no "fetch all groups"
// usync, so there is no live way to enumerate joined groups in this build: we
// require an active session (so callers get a connect error rather than a silent
// empty when offline) and then return an empty list. Documented limitation —
// callers wanting a specific group should use GroupMetadata with its JID.
func (b *ManagerBackend) Groups(ctx context.Context, name string) ([]GroupArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	infos, err := c.FetchAllGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GroupArg, 0, len(infos))
	for _, info := range infos {
		parts := make([]GroupParticipantArg, 0, len(info.Participants))
		for _, p := range info.Participants {
			parts = append(parts, GroupParticipantArg{JID: p.JID, Admin: participantAdmin(p.IsAdmin, p.IsSuperAdmin)})
		}
		out = append(out, GroupArg{
			JID:          info.JID,
			Subject:      info.Subject,
			Owner:        info.Owner,
			Desc:         info.Desc,
			Creation:     info.Creation,
			Participants: parts,
		})
	}
	return out, nil
}

// GroupMetadata returns metadata for one group via a live w:g2 query, mapping the
// library's GroupInfo (exported fields only) into the backend-neutral GroupArg.
func (b *ManagerBackend) GroupMetadata(ctx context.Context, name, jid string) (GroupArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return GroupArg{}, err
	}
	info, err := c.GroupMetadata(ctx, jid)
	if err != nil {
		return GroupArg{}, err
	}
	parts := make([]GroupParticipantArg, 0, len(info.Participants))
	for _, p := range info.Participants {
		parts = append(parts, GroupParticipantArg{JID: p.JID, Admin: participantAdmin(p.IsAdmin, p.IsSuperAdmin)})
	}
	return GroupArg{
		JID:          info.JID,
		Subject:      info.Subject,
		Owner:        info.Owner,
		Desc:         info.Desc,
		Creation:     info.Creation,
		Participants: parts,
	}, nil
}

// SendPresence sets typing (composing|paused, per-chat) or global availability
// (available|unavailable) through the live client. The presence string is passed
// as an untyped constant so the library's named PresenceState/ChatState types
// need not be imported by this module.
func (b *ManagerBackend) SendPresence(ctx context.Context, name, jid, presence string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	switch presence {
	case "composing":
		return c.SendTyping(ctx, jid, "composing")
	case "paused":
		return c.SendTyping(ctx, jid, "paused")
	case "available":
		return c.SendPresence(ctx, "available")
	case "unavailable":
		return c.SendPresence(ctx, "unavailable")
	default:
		return fmt.Errorf("unsupported presence %q", presence)
	}
}

// MarkRead sends read receipts for the given message ids on a chat. The
// participant argument is left empty (1:1 chats); group read receipts would need
// the sender's jid which this build does not thread through.
func (b *ManagerBackend) MarkRead(ctx context.Context, name, jid string, ids []string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.SendReadReceipt(ctx, jid, ids, "")
}

// GroupCreate creates a new group through the live client and maps the returned
// GroupInfo into the backend-neutral GroupArg.
func (b *ManagerBackend) GroupCreate(ctx context.Context, name, subject string, participants []string) (GroupArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return GroupArg{}, err
	}
	info, err := c.GroupCreate(ctx, subject, participants)
	if err != nil {
		return GroupArg{}, err
	}
	parts := make([]GroupParticipantArg, 0, len(info.Participants))
	for _, p := range info.Participants {
		parts = append(parts, GroupParticipantArg{JID: p.JID, Admin: participantAdmin(p.IsAdmin, p.IsSuperAdmin)})
	}
	return GroupArg{
		JID:          info.JID,
		Subject:      info.Subject,
		Owner:        info.Owner,
		Desc:         info.Desc,
		Creation:     info.Creation,
		Participants: parts,
	}, nil
}

// GroupUpdateParticipants adds/removes/promotes/demotes members through the live
// client and maps the per-participant result.
func (b *ManagerBackend) GroupUpdateParticipants(ctx context.Context, name, groupJID, action string, participants []string) ([]ParticipantResult, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	res, err := c.GroupParticipantsUpdate(ctx, groupJID, participants, action)
	if err != nil {
		return nil, err
	}
	out := make([]ParticipantResult, 0, len(res))
	for _, r := range res {
		out = append(out, ParticipantResult{JID: r.JID, Status: r.Status})
	}
	return out, nil
}

// GroupInviteCode returns the group's current invite code through the live client.
func (b *ManagerBackend) GroupInviteCode(ctx context.Context, name, groupJID string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.GroupInviteCode(ctx, groupJID)
}

// GroupLeave leaves a group through the live client.
func (b *ManagerBackend) GroupLeave(ctx context.Context, name, groupJID string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.GroupLeave(ctx, groupJID)
}

// participantAdmin maps the library's two admin booleans to the Evolution admin
// string ("superadmin"|"admin"|"").
func participantAdmin(isAdmin, isSuperAdmin bool) string {
	switch {
	case isSuperAdmin:
		return "superadmin"
	case isAdmin:
		return "admin"
	default:
		return ""
	}
}

// connectionStatus maps a wa.State to the Evolution connectionStatus value.
func connectionStatus(s wa.State) string {
	switch s {
	case wa.StateLoggedIn:
		return "open"
	case wa.StateConnecting, wa.StateConnected, wa.StateBackoff:
		return "connecting"
	default:
		return "close"
	}
}

// normalizeJID appends @s.whatsapp.net to a bare phone number (no domain). A
// value already containing "@" is returned unchanged. Leading "+" is stripped.
func normalizeJID(number string) string {
	number = strings.TrimSpace(number)
	if number == "" {
		return ""
	}
	if strings.Contains(number, "@") {
		return number
	}
	number = strings.TrimPrefix(number, "+")
	return number + "@s.whatsapp.net"
}
