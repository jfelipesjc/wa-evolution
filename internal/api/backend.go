package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wa "github.com/jfelipesjc/wa-go/wa"
)

// Backend is the seam between the HTTP handlers and the WhatsApp session stack.
// Handlers depend only on this interface, so the whole API surface is testable
// offline with a fake (no Manager, no Noise handshake, no network). The
// production implementation (ManagerBackend) adapts *wa.Manager.
type Backend interface {
	// Create registers a new instance (opens its store, adds it to the manager).
	// It pairs by QR.
	Create(name string) error
	// CreateWithNumber registers a new instance that pairs via pairing CODE for the
	// given phone number (returns the 8-char code through PairingCode) instead of
	// QR. An empty number behaves like Create.
	CreateWithNumber(name, number string) error
	// Connect starts the instance and returns the latest QR code string captured
	// from its event stream (empty if already logged in / none yet).
	Connect(ctx context.Context, name string) (qr string, err error)
	// PairingCode returns the latest pairing code captured for an instance (empty
	// if the instance pairs by QR / none yet). Populated for number-paired instances.
	PairingCode(name string) string
	// RequestPairingCode asks WhatsApp for an 8-char pairing code for the given
	// phone number on the instance's live (QR) pairing session, returning the code
	// to type on the phone. Errors if the instance has no active pairing session.
	RequestPairingCode(ctx context.Context, name, number string) (string, error)
	// Delete removes an instance (stops + closes its store).
	Delete(name string) error
	// Logout disconnects an instance without deleting its store.
	Logout(name string) error
	// Status returns name -> Evolution connectionStatus (open|connecting|close).
	Status() map[string]string
	// Exists reports whether the named instance is registered.
	Exists(name string) bool
	// OwnProfile returns the instance's own account number (digits) and display
	// name (pushName) from its stored creds. Empty strings if not paired yet.
	OwnProfile(name string) (number, pushName string)
	// PairingNumber returns the phone number an instance was created to pair by
	// CODE with ("" when it pairs by QR). The UI uses it to gate the pairing-code
	// option.
	PairingNumber(name string) string

	// SendText sends a text message; returns the message id.
	SendText(ctx context.Context, name, jid, text string) (string, error)
	// SendTextReply sends a text quoting an earlier message (q identifies it);
	// returns the message id.
	SendTextReply(ctx context.Context, name, jid, text string, q QuotedRef) (string, error)
	// SendMedia sends an image/video/audio/document; returns the message id.
	SendMedia(ctx context.Context, name, jid string, m MediaArg) (string, error)
	// SendReaction reacts to a target message; returns the reaction message id.
	SendReaction(ctx context.Context, name, jid, msgID string, fromMe bool, emoji string) (string, error)
	// DeleteMessage revokes (deletes for everyone) a target message; returns the
	// revoke message id.
	DeleteMessage(ctx context.Context, name, jid, msgID string, fromMe bool) (string, error)
	// EditMessage edits a previously sent text message; returns the edit message id.
	EditMessage(ctx context.Context, name, jid, msgID string, fromMe bool, text string) (string, error)
	// SendButtons sends a quick-reply buttons message; returns the message id.
	SendButtons(ctx context.Context, name, jid, text, footer string, buttonIDs, buttonTexts []string) (string, error)
	// SendList sends a single-select list message; returns the message id.
	SendList(ctx context.Context, name, jid, text, buttonText string, sectionTitles []string, rowTitles, rowDescs, rowIDs [][]string) (string, error)
	// SendLocation sends a location pin; returns the message id.
	SendLocation(ctx context.Context, name, jid string, lat, lng float64, locName, address string) (string, error)
	// SendContact sends a vCard contact; returns the message id.
	SendContact(ctx context.Context, name, jid, displayName, vcard string) (string, error)

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

	// --- profile / status / newsletter ---
	// ProfileSetName updates the account's display name.
	ProfileSetName(ctx context.Context, name, displayName string) error
	// ProfileSetStatus updates the account's "about" / status text.
	ProfileSetStatus(ctx context.Context, name, status string) error
	// PostStatus posts a text status (story) to the given recipient JIDs.
	PostStatus(ctx context.Context, name, text string, recipients []string) (string, error)
	// NewsletterCreate creates a channel and returns its JID.
	NewsletterCreate(ctx context.Context, name, channelName, description string) (string, error)
	// NewsletterFollow follows a channel by JID.
	NewsletterFollow(ctx context.Context, name, jid string) error

	// --- extended parity surface (Evolution routers) ---

	// Restart best-effort cycles an instance's connection.
	Restart(name string) error
	// SendPoll sends a poll; returns the message id.
	SendPoll(ctx context.Context, name, jid, pollName string, options []string, selectableCount int) (string, error)
	// SendSticker sends a sticker (webp bytes); returns the message id.
	SendSticker(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error)
	// SendWhatsAppAudio sends a PTT voice note (audio bytes); returns the message id.
	SendWhatsAppAudio(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error)
	// SendPtv sends a round video note (PTV); returns the message id.
	SendPtv(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error)

	// ArchiveChat archives/unarchives a chat (app-state).
	ArchiveChat(ctx context.Context, name, jid string, archive bool) error
	// FetchProfilePictureURL returns a contact/group profile picture URL.
	FetchProfilePictureURL(ctx context.Context, name, jid string, preview bool) (string, error)
	// FetchBusinessProfile returns a business profile.
	FetchBusinessProfile(ctx context.Context, name, jid string) (BusinessProfileArg, error)
	// FetchPrivacy returns the account's privacy settings.
	FetchPrivacy(ctx context.Context, name string) (map[string]string, error)
	// UpdatePrivacy sets one privacy toggle (setting -> value).
	UpdatePrivacy(ctx context.Context, name, setting, value string) error
	// UpdateBlockStatus blocks/unblocks a contact.
	UpdateBlockStatus(ctx context.Context, name, jid string, block bool) error
	// UpdateProfilePicture sets a profile/group picture (jid empty = self).
	UpdateProfilePicture(ctx context.Context, name, jid string, jpeg []byte) error
	// RemoveProfilePicture removes a profile/group picture (jid empty = self).
	RemoveProfilePicture(ctx context.Context, name, jid string) error
	// FetchProfile returns a composed profile (picture + status) for a JID.
	FetchProfile(ctx context.Context, name, jid string) (ProfileArg, error)
	// FindChats lists materialized chats from the instance's ChatStore.
	FindChats(name string) ([]ChatInfoArg, error)
	// FindChatByRemoteJID returns one chat from the ChatStore.
	FindChatByRemoteJID(name, jid string) (ChatInfoArg, bool, error)
	// FindContacts lists materialized contacts from the instance's ChatStore.
	FindContacts(name string) ([]ContactArg, error)

	// GroupAcceptInvite joins a group by invite code; returns the group JID.
	GroupAcceptInvite(ctx context.Context, name, code string) (string, error)
	// GroupInviteInfo returns metadata for an invite code (preview before joining).
	GroupInviteInfo(ctx context.Context, name, code string) (GroupArg, error)
	// GroupRevokeInvite resets a group's invite code; returns the new code.
	GroupRevokeInvite(ctx context.Context, name, groupJID string) (string, error)
	// GroupToggleEphemeral sets disappearing-message duration (seconds, 0 = off).
	GroupToggleEphemeral(ctx context.Context, name, groupJID string, seconds int) error
	// GroupUpdateDescription sets a group's description.
	GroupUpdateDescription(ctx context.Context, name, groupJID, desc string) error
	// GroupUpdateSubject sets a group's subject (name).
	GroupUpdateSubject(ctx context.Context, name, groupJID, subject string) error
	// GroupUpdatePicture sets a group's picture (jpeg bytes).
	GroupUpdatePicture(ctx context.Context, name, groupJID string, jpeg []byte) error
	// GroupUpdateSetting changes a group setting (announcement|not_announcement|locked|unlocked).
	GroupUpdateSetting(ctx context.Context, name, groupJID, setting string) error
	// GroupSendInvite messages an invite link to recipients.
	GroupSendInvite(ctx context.Context, name, groupJID string, numbers []string, text string) error

	// GetLabels returns the account's defined labels.
	GetLabels(ctx context.Context, name string) ([]LabelArg, error)
	// HandleLabel adds/removes a label on a chat (action add|remove).
	HandleLabel(ctx context.Context, name, chatJID, labelID, action string) error

	// OfferCall rings a contact (signaling only); returns the call id.
	OfferCall(ctx context.Context, name, jid string, video bool) (string, error)
	// GetCatalog returns a business catalog (products).
	GetCatalog(ctx context.Context, name, jid string, limit int) ([]ProductArg, error)
	// GetBase64FromMedia downloads a stored media message's plaintext bytes,
	// returning them with the media mimetype. Backs getBase64FromMediaMessage.
	GetBase64FromMedia(ctx context.Context, name, jid, msgID string) ([]byte, string, error)
	// MarkChatUnread marks a whole chat as unread (app-state).
	MarkChatUnread(ctx context.Context, name, jid string) error

	// PinChat pins/unpins a chat (app-state).
	PinChat(ctx context.Context, name, jid string, pin bool) error
	// MuteChat mutes a chat for the given duration in seconds (0 = unmute).
	MuteChat(ctx context.Context, name, jid string, seconds int) error
	// StarMessage stars/unstars a single message in a chat (app-state).
	StarMessage(ctx context.Context, name, jid, msgID string, fromMe, star bool) error
	// ClearChat clears a chat's messages for this account, keeping the chat (app-state).
	ClearChat(ctx context.Context, name, jid string) error
	// DeleteChat deletes a chat for this account (app-state).
	DeleteChat(ctx context.Context, name, jid string) error
	// ResyncAppState fetches and applies the server's app-state for the given
	// collections (fresh requests a full snapshot).
	ResyncAppState(ctx context.Context, name string, collections []string, fresh bool) error

	// --- communities ---

	// CommunityCreate creates a community and returns its metadata.
	CommunityCreate(ctx context.Context, name, subject, description string) (*wa.GroupInfo, error)
	// CommunityMetadata returns a community's metadata (including participants).
	CommunityMetadata(ctx context.Context, name, communityJID string) (*wa.GroupInfo, error)
	// CommunityUpdateSubject sets a community's subject (name).
	CommunityUpdateSubject(ctx context.Context, name, communityJID, subject string) error
	// CommunityUpdateDescription sets a community's description.
	CommunityUpdateDescription(ctx context.Context, name, communityJID, description string) error
	// CommunityLinkGroups links one or more groups into a community, returning a
	// per-group outcome.
	CommunityLinkGroups(ctx context.Context, name, communityJID string, groupJIDs []string) ([]CommunityLinkResult, error)
	// CommunityUnlinkGroup unlinks a group from a community.
	CommunityUnlinkGroup(ctx context.Context, name, communityJID, groupJID string) error
	// CommunityLinkedGroups lists the groups linked into a community.
	CommunityLinkedGroups(ctx context.Context, name, communityJID string) ([]wa.GroupLinkInfo, error)
	// CommunityRequestList lists pending membership (join) requests for a community.
	CommunityRequestList(ctx context.Context, name, communityJID string) ([]wa.CommunityMembershipRequest, error)
	// CommunityRequestUpdate approves/rejects pending membership requests.
	CommunityRequestUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error)
	// CommunityParticipantsUpdate adds/removes/promotes/demotes community members.
	CommunityParticipantsUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error)
	// CommunityJoinApprovalMode toggles join-approval (on|off).
	CommunityJoinApprovalMode(ctx context.Context, name, communityJID, mode string) error
	// CommunityMemberAddMode sets who can add members (admin_add|all_member_add).
	CommunityMemberAddMode(ctx context.Context, name, communityJID, mode string) error
	// CommunityToggleEphemeral sets disappearing-message duration (seconds, 0 = off).
	CommunityToggleEphemeral(ctx context.Context, name, communityJID string, expiration int) error
	// CommunitySettingUpdate changes a community setting (lib setting tag).
	CommunitySettingUpdate(ctx context.Context, name, communityJID, setting string) error
	// CommunityLeave leaves a community.
	CommunityLeave(ctx context.Context, name, communityJID string) error
	// CommunityFetchAllParticipating lists the communities the account participates
	// in (jid + subject per community).
	CommunityFetchAllParticipating(ctx context.Context, name string) ([]wa.GroupLinkInfo, error)
	// CommunityCreateGroup creates a new sub-group under a community and returns
	// its metadata.
	CommunityCreateGroup(ctx context.Context, name, communityJID, subject string, participants []string) (*wa.GroupInfo, error)
	// CommunityLinkedGroupsParticipants lists the JIDs of every participant across
	// a community's linked sub-groups.
	CommunityLinkedGroupsParticipants(ctx context.Context, name, communityJID string) ([]string, error)

	// --- newsletter admin / metadata ---

	// NewsletterMetadata returns a channel's metadata by JID or invite key.
	// keyType is "jid" or "invite".
	NewsletterMetadata(ctx context.Context, name, key, keyType string) (*wa.NewsletterInfo, error)
	// NewsletterUnfollow unfollows a channel.
	NewsletterUnfollow(ctx context.Context, name, jid string) error
	// NewsletterMute mutes (mute=true) or unmutes (mute=false) a channel.
	NewsletterMute(ctx context.Context, name, jid string, mute bool) error
	// NewsletterUpdateName sets a channel's name; returns the updated metadata.
	NewsletterUpdateName(ctx context.Context, name, jid, newName string) (*wa.NewsletterInfo, error)
	// NewsletterUpdateDescription sets a channel's description; returns the updated metadata.
	NewsletterUpdateDescription(ctx context.Context, name, jid, desc string) (*wa.NewsletterInfo, error)
	// NewsletterUpdatePicture sets a channel's picture (base64 jpeg, "" removes);
	// returns the updated metadata.
	NewsletterUpdatePicture(ctx context.Context, name, jid, picture string) (*wa.NewsletterInfo, error)
	// NewsletterReactionMode sets the channel-wide reaction policy
	// (ALL|BASIC|NONE|BLOCKLIST).
	NewsletterReactionMode(ctx context.Context, name, jid, mode string) error
	// NewsletterFetchMessages fetches up to count messages since the given server
	// id (0 = newest), starting after the given server id (0 = no cursor).
	NewsletterFetchMessages(ctx context.Context, name, jid string, count int, since int64, after int64) ([]wa.NewsletterMessage, error)
	// NewsletterAdminCount returns the number of admins on a channel.
	NewsletterAdminCount(ctx context.Context, name, jid string) (int, error)
	// NewsletterChangeOwner transfers channel ownership to newOwnerJid.
	NewsletterChangeOwner(ctx context.Context, name, jid, newOwnerJid string) error
	// NewsletterDemote demotes an admin (userJid) back to subscriber.
	NewsletterDemote(ctx context.Context, name, jid, userJid string) error
	// NewsletterSubscribeLiveUpdates subscribes to a channel's live updates and
	// returns the granted duration.
	NewsletterSubscribeLiveUpdates(ctx context.Context, name, jid string) (string, error)
	// NewsletterDelete permanently deactivates (deletes) a channel. IRREVERSIBLE.
	NewsletterDelete(ctx context.Context, name, jid string) error
	// NewsletterSubscriberCount returns a channel's subscriber count.
	NewsletterSubscriberCount(ctx context.Context, name, jid string) (int, error)
	// NewsletterReactMessage reacts to a channel message (emoji ""=remove).
	NewsletterReactMessage(ctx context.Context, name, jid, serverID, reaction string) error
	// SendNewsletterText posts a text message to a channel and returns its
	// server-assigned id (server_id).
	SendNewsletterText(ctx context.Context, name, jid, text string) (string, error)
	// NewsletterSubscribed lists the channels the account follows or owns.
	NewsletterSubscribed(ctx context.Context, name string) ([]*wa.NewsletterInfo, error)
	// AcceptTOSNotice accepts the WhatsApp Channels TOS notice.
	AcceptTOSNotice(ctx context.Context, name string) error
	// NewsletterMarkViewed bumps the view counter of one or more channel messages.
	NewsletterMarkViewed(ctx context.Context, name, jid string, serverIDs []string) error
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
	name     string
	store    wa.Store
	mc       *wa.ManagedClient
	chats    *wa.ChatStore
	qr         string // latest QR code captured from the event stream
	pairCode   string // latest pairing code captured (number-paired instances)
	pairNumber string // the phone number this instance pairs by code with ("" = QR)
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

// Restore re-registers every instance persisted under the data dir so paired
// sessions survive a server restart: it scans for <name>.db files and re-adds
// each (the store reloads the saved creds, so the manager reconnects without a
// re-pair). Returns the names restored. Call once at startup before serving.
func (b *ManagerBackend) Restore() ([]string, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("read data dir: %w", err)
	}
	var restored []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		// Only the primary SQLite file; skip -wal/-shm sidecars and .webhook files.
		if !strings.HasSuffix(fn, ".db") {
			continue
		}
		name := strings.TrimSuffix(fn, ".db")
		if name == "" || b.Exists(name) {
			continue
		}
		// Restored instances are already paired (creds in the store) — they log in
		// via the normal connect path, so no pairing number is needed.
		if err := b.Create(name); err != nil {
			return restored, fmt.Errorf("restore %q: %w", name, err)
		}
		restored = append(restored, name)
	}
	return restored, nil
}

// ChatStore returns the per-instance ChatStore (for the event pump's feed). It
// returns nil for unknown instances.
func (b *ManagerBackend) ChatStore(name string) *wa.ChatStore { return b.chatStore(name) }

// SetQR records the latest QR code for an instance (called by the event pump).
func (b *ManagerBackend) SetQR(name, code string) { b.setQR(name, code) }

// SetPairingCode records the latest pairing code for an instance (called by the
// event pump when a PairingCodeEvent arrives for a number-paired instance).
func (b *ManagerBackend) SetPairingCode(name, code string) {
	if in, ok := b.get(name); ok {
		b.mu.Lock()
		in.pairCode = code
		b.mu.Unlock()
	}
}

// PairingCode returns the latest captured pairing code for an instance.
func (b *ManagerBackend) PairingCode(name string) string {
	if in, ok := b.get(name); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		return in.pairCode
	}
	return ""
}

// RequestPairingCode switches the instance into pairing-CODE mode for the given
// number (reconnecting via ConnectWithPairingCode, the proven path) and waits for
// the emitted 8-char code to be captured. The number is supplied/confirmed by the
// user at connect time.
func (b *ManagerBackend) RequestPairingCode(ctx context.Context, name, number string) (string, error) {
	number = sanitizeNumber(number)
	if number == "" {
		return "", errors.New("number is required")
	}
	if !b.Exists(name) {
		return "", ErrInstanceNotFound
	}
	b.SetPairingCode(name, "") // drop any stale code before reconnecting
	if err := b.mgr.SetPairingNumber(name, number); err != nil {
		return "", err
	}
	// ConnectWithPairingCode emits a PairingCodeEvent which the event pump captures
	// into pairCode; poll for it.
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if code := b.PairingCode(name); code != "" {
			return code, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return "", errors.New("timed out waiting for pairing code")
}

// PairingNumber returns the number an instance pairs by code with ("" = QR).
func (b *ManagerBackend) PairingNumber(name string) string {
	if in, ok := b.get(name); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		return in.pairNumber
	}
	return ""
}

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

// OwnProfile reads the instance's own number + display name from its stored creds.
func (b *ManagerBackend) OwnProfile(name string) (number, pushName string) {
	in, ok := b.get(name)
	if !ok {
		return "", ""
	}
	creds, ok, err := in.store.LoadCreds()
	if err != nil || !ok || creds == nil {
		return "", ""
	}
	// creds.Me is a JID like "5512999999999:3@s.whatsapp.net" or with @s.whatsapp.net.
	number = creds.Me
	if i := strings.IndexAny(number, ":@"); i >= 0 {
		number = number[:i]
	}
	return number, creds.PushName
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

func (b *ManagerBackend) Create(name string) error { return b.createInternal(name, "") }

// CreateWithNumber registers an instance that pairs by code for the given number.
func (b *ManagerBackend) CreateWithNumber(name, number string) error {
	return b.createInternal(name, number)
}

func (b *ManagerBackend) createInternal(name, number string) error {
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
	// Always connect in QR mode (active pairing session). The pairing CODE is then
	// requested on demand for a user-supplied number via RequestPairingCode (the
	// number passed at create time is kept only as a default/hint for the UI).
	number = sanitizeNumber(number)
	mc, err := b.mgr.Add(name, st)
	if err != nil {
		_ = st.Close()
		return err
	}
	b.mu.Lock()
	b.instances[name] = &mbInstance{name: name, store: st, mc: mc, chats: wa.NewChatStore(), pairNumber: number}
	b.mu.Unlock()
	return nil
}

// sanitizeNumber strips everything but digits from a phone number (the pairing
// flow wants a bare international number, e.g. 5512999998888).
func sanitizeNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (b *ManagerBackend) Connect(ctx context.Context, name string) (string, error) {
	in, ok := b.get(name)
	if !ok {
		return "", ErrInstanceNotFound
	}
	// Ensure QR mode (no-op unless the instance was switched to code mode by a
	// prior pairing-code request) so /connect yields a QR, and drop any cached
	// pairing code so it can't be mistaken for a QR response.
	_ = b.mgr.SetPairingNumber(name, "")
	b.SetPairingCode(name, "")
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
	// Stop + unregister the instance in the manager first (ends its connection and
	// event pump) so it no longer shows in Status as a zombie; then close its store.
	_ = b.mgr.Remove(name)
	closeErr := in.store.Close()
	// Remove the on-disk store files so a deleted instance does NOT resurrect on the
	// next Restore() (boot). Covers the SQLite primary + WAL/SHM sidecars.
	base := filepath.Join(b.dir, name+".db")
	for _, p := range []string{base, base + "-wal", base + "-shm", base + "-journal"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			closeErr = err
		}
	}
	return closeErr
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
// QuotedRef identifies a WhatsApp message to quote in a reply.
type QuotedRef struct {
	ID          string
	RemoteJID   string
	FromMe      bool
	Text        string
	Participant string
}

// SendTextReply sends a text reply quoting an earlier message.
func (b *ManagerBackend) SendTextReply(ctx context.Context, name, jid, text string, q QuotedRef) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendTextReplyTo(ctx, jid, text, q.ID, q.RemoteJID, q.FromMe, q.Text, q.Participant)
}

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

// DeleteMessage revokes a target message through the live client.
func (b *ManagerBackend) DeleteMessage(ctx context.Context, name, jid, msgID string, fromMe bool) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.DeleteMessageByID(ctx, jid, msgID, fromMe)
}

// EditMessage edits a previously sent text message through the live client.
func (b *ManagerBackend) EditMessage(ctx context.Context, name, jid, msgID string, fromMe bool, text string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.EditTextByID(ctx, jid, msgID, fromMe, text)
}

// SendButtons sends a quick-reply buttons message through the live client.
func (b *ManagerBackend) SendButtons(ctx context.Context, name, jid, text, footer string, buttonIDs, buttonTexts []string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendButtonsSimple(ctx, jid, text, footer, buttonIDs, buttonTexts)
}

// SendList sends a single-select list message through the live client.
func (b *ManagerBackend) SendList(ctx context.Context, name, jid, text, buttonText string, sectionTitles []string, rowTitles, rowDescs, rowIDs [][]string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendListSimple(ctx, jid, text, buttonText, sectionTitles, rowTitles, rowDescs, rowIDs)
}

// SendLocation sends a location pin through the live client.
func (b *ManagerBackend) SendLocation(ctx context.Context, name, jid string, lat, lng float64, locName, address string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendLocation(ctx, jid, lat, lng, locName, address)
}

// SendContact sends a vCard contact through the live client.
func (b *ManagerBackend) SendContact(ctx context.Context, name, jid, displayName, vcard string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendContact(ctx, jid, displayName, vcard)
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
