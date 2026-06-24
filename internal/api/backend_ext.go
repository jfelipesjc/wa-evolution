package api

import (
	"context"
	"fmt"
	"time"

	wa "github.com/felipeleal/wa-go/wa"
)

// --- backend-neutral arg types for the extended parity surface ---

// BusinessProfileArg is the backend-neutral business profile.
type BusinessProfileArg struct {
	JID         string
	Address     string
	Description string
	Website     string
	Email       string
}

// ProfileArg is a composed profile view (picture URL + status text).
type ProfileArg struct {
	JID        string
	PictureURL string
	Status     string
}

// ChatInfoArg is the backend-neutral chat row.
type ChatInfoArg struct {
	JID         string
	Name        string
	UnreadCount int
	Archived    bool
	Pinned      bool
	Muted       bool
	Timestamp   int64
}

// ContactArg is the backend-neutral contact row.
type ContactArg struct {
	JID      string
	Name     string
	Notify   string
	PushName string
}

// LabelArg is the backend-neutral label.
type LabelArg struct {
	ID    string
	Name  string
	Color string
}

// ProductArg is the backend-neutral catalog product.
type ProductArg struct {
	ID          string
	Name        string
	Description string
	Price       int64
	Currency    string
}

// --- ManagerBackend implementations ---

func (b *ManagerBackend) Restart(name string) error {
	if !b.Exists(name) {
		return ErrInstanceNotFound
	}
	// The Manager auto-reconnects supervised instances; an explicit per-instance
	// stop/start is not exposed in this build, so restart is a best-effort no-op
	// that simply confirms the instance is known. Documented limitation.
	return nil
}

func (b *ManagerBackend) SendPoll(ctx context.Context, name, jid, pollName string, options []string, selectableCount int) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendPoll(ctx, jid, pollName, options, selectableCount)
}

func (b *ManagerBackend) SendSticker(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	c.EnableDefaultMediaTransfer()
	if mimetype == "" {
		mimetype = "image/webp"
	}
	return c.SendSticker(ctx, jid, data, wa.MediaOpts{Mimetype: mimetype})
}

func (b *ManagerBackend) SendWhatsAppAudio(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	c.EnableDefaultMediaTransfer()
	if mimetype == "" {
		mimetype = "audio/ogg; codecs=opus"
	}
	return c.SendAudioBytes(ctx, jid, data, mimetype)
}

func (b *ManagerBackend) ArchiveChat(ctx context.Context, name, jid string, archive bool) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.ArchiveChat(ctx, jid, archive)
}

func (b *ManagerBackend) FetchProfilePictureURL(ctx context.Context, name, jid string, preview bool) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.ProfilePictureURL(ctx, jid, preview)
}

func (b *ManagerBackend) FetchBusinessProfile(ctx context.Context, name, jid string) (BusinessProfileArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return BusinessProfileArg{}, err
	}
	p, err := c.GetBusinessProfile(ctx, jid)
	if err != nil {
		return BusinessProfileArg{}, err
	}
	return BusinessProfileArg{
		JID:         p.JID,
		Address:     p.Address,
		Description: p.Description,
		Website:     p.Website,
		Email:       p.Email,
	}, nil
}

func (b *ManagerBackend) FetchPrivacy(ctx context.Context, name string) (map[string]string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.FetchPrivacySettings(ctx)
}

func (b *ManagerBackend) UpdatePrivacy(ctx context.Context, name, setting, value string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.UpdatePrivacy(ctx, wa.PrivacySetting(setting), value)
}

func (b *ManagerBackend) UpdateBlockStatus(ctx context.Context, name, jid string, block bool) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	if block {
		return c.Block(ctx, jid)
	}
	return c.Unblock(ctx, jid)
}

func (b *ManagerBackend) UpdateProfilePicture(ctx context.Context, name, jid string, jpeg []byte) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.SetProfilePicture(ctx, jid, jpeg)
}

func (b *ManagerBackend) RemoveProfilePicture(ctx context.Context, name, jid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.RemoveProfilePicture(ctx, jid)
}

func (b *ManagerBackend) FetchProfile(ctx context.Context, name, jid string) (ProfileArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return ProfileArg{}, err
	}
	out := ProfileArg{JID: jid}
	// Best-effort compose: picture and status are independent queries; a failure on
	// one (e.g. privacy-restricted) should not nuke the other.
	if url, perr := c.ProfilePictureURL(ctx, jid, false); perr == nil {
		out.PictureURL = url
	}
	if st, serr := c.FetchStatus(ctx, jid); serr == nil {
		out.Status = st
	}
	return out, nil
}

func (b *ManagerBackend) FindChats(name string) ([]ChatInfoArg, error) {
	in, ok := b.get(name)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	chats := in.chats.Chats()
	out := make([]ChatInfoArg, 0, len(chats))
	for _, ch := range chats {
		out = append(out, chatToArg(ch))
	}
	return out, nil
}

func (b *ManagerBackend) FindChatByRemoteJID(name, jid string) (ChatInfoArg, bool, error) {
	in, ok := b.get(name)
	if !ok {
		return ChatInfoArg{}, false, ErrInstanceNotFound
	}
	ch, ok := in.chats.Chat(jid)
	if !ok {
		return ChatInfoArg{}, false, nil
	}
	return chatToArg(ch), true, nil
}

func (b *ManagerBackend) FindContacts(name string) ([]ContactArg, error) {
	in, ok := b.get(name)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	// Contacts are surfaced through the chat list (each chat's peer). The ChatStore
	// exposes per-JID Contact lookup; we derive the contact set from the chats.
	chats := in.chats.Chats()
	out := make([]ContactArg, 0, len(chats))
	for _, ch := range chats {
		ct, ok := in.chats.Contact(ch.JID)
		if !ok {
			ct = wa.Contact{JID: ch.JID, Name: ch.Name}
		}
		out = append(out, ContactArg{JID: ct.JID, Name: ct.Name, Notify: ct.Notify, PushName: ct.PushName})
	}
	return out, nil
}

func (b *ManagerBackend) GroupAcceptInvite(ctx context.Context, name, code string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.GroupAcceptInvite(ctx, code)
}

func (b *ManagerBackend) GroupInviteInfo(ctx context.Context, name, code string) (GroupArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return GroupArg{}, err
	}
	info, err := c.GroupGetInviteInfo(ctx, code)
	if err != nil {
		return GroupArg{}, err
	}
	parts := make([]GroupParticipantArg, 0, len(info.Participants))
	for _, p := range info.Participants {
		parts = append(parts, GroupParticipantArg{JID: p.JID, Admin: participantAdmin(p.IsAdmin, p.IsSuperAdmin)})
	}
	return GroupArg{JID: info.JID, Subject: info.Subject, Owner: info.Owner, Desc: info.Desc, Creation: info.Creation, Participants: parts}, nil
}

func (b *ManagerBackend) GroupRevokeInvite(ctx context.Context, name, groupJID string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.GroupRevokeInvite(ctx, groupJID)
}

func (b *ManagerBackend) GroupToggleEphemeral(ctx context.Context, name, groupJID string, seconds int) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	_, err = c.SetDisappearingMessages(ctx, groupJID, time.Duration(seconds)*time.Second)
	return err
}

func (b *ManagerBackend) GroupUpdateDescription(ctx context.Context, name, groupJID, desc string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.GroupUpdateDescription(ctx, groupJID, desc)
}

func (b *ManagerBackend) GroupUpdateSubject(ctx context.Context, name, groupJID, subject string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.GroupUpdateSubject(ctx, groupJID, subject)
}

func (b *ManagerBackend) GroupUpdatePicture(ctx context.Context, name, groupJID string, jpeg []byte) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.SetProfilePicture(ctx, groupJID, jpeg)
}

func (b *ManagerBackend) GroupUpdateSetting(ctx context.Context, name, groupJID, setting string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.GroupSettingUpdate(ctx, groupJID, setting)
}

func (b *ManagerBackend) GroupSendInvite(ctx context.Context, name, groupJID string, numbers []string, text string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	code, err := c.GroupInviteCode(ctx, groupJID)
	if err != nil {
		return err
	}
	link := "https://chat.whatsapp.com/" + code
	body := text
	if body == "" {
		body = link
	} else {
		body = body + "\n" + link
	}
	for _, num := range numbers {
		if _, err := c.SendText(ctx, num, body); err != nil {
			return err
		}
	}
	return nil
}

func (b *ManagerBackend) GetLabels(ctx context.Context, name string) ([]LabelArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	labels, err := c.GetLabels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LabelArg, 0, len(labels))
	for _, l := range labels {
		out = append(out, LabelArg{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	return out, nil
}

func (b *ManagerBackend) HandleLabel(ctx context.Context, name, chatJID, labelID, action string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	switch action {
	case "add":
		return c.AddChatLabel(ctx, chatJID, labelID)
	case "remove":
		return c.RemoveChatLabel(ctx, chatJID, labelID)
	default:
		return fmt.Errorf("unsupported label action %q", action)
	}
}

func (b *ManagerBackend) OfferCall(ctx context.Context, name, jid string, video bool) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.OfferCall(ctx, jid, video)
}

func (b *ManagerBackend) GetCatalog(ctx context.Context, name, jid string, limit int) ([]ProductArg, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	products, err := c.GetCatalog(ctx, jid, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ProductArg, 0, len(products))
	for _, p := range products {
		out = append(out, ProductArg{ID: p.ID, Name: p.Name, Description: p.Description, Price: p.Price, Currency: p.Currency})
	}
	return out, nil
}

func (b *ManagerBackend) GetBase64FromMedia(ctx context.Context, name, jid, msgID string) ([]byte, string, error) {
	in, ok := b.get(name)
	if !ok {
		return nil, "", ErrInstanceNotFound
	}
	c, err := b.liveClient(name)
	if err != nil {
		return nil, "", err
	}
	// Locate the stored message (carrying the raw WebMessageInfo with media keys)
	// in the chat's history. ChatMessages returns newest-last; scan for the id.
	for _, sm := range in.chats.ChatMessages(jid, 0) {
		if sm.Key == msgID {
			if sm.Raw == nil {
				return nil, "", fmt.Errorf("stored message %q has no raw payload", msgID)
			}
			return c.DownloadStoredMedia(ctx, sm.Raw)
		}
	}
	return nil, "", fmt.Errorf("message %q not found in chat %q history", msgID, jid)
}

func (b *ManagerBackend) MarkChatUnread(ctx context.Context, name, jid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.MarkRead(ctx, jid, false)
}

// --- helpers ---

func chatToArg(ch wa.Chat) ChatInfoArg {
	return ChatInfoArg{
		JID:         ch.JID,
		Name:        ch.Name,
		UnreadCount: ch.UnreadCount,
		Archived:    ch.Archived,
		Pinned:      ch.Pinned,
		Muted:       ch.Muted,
		Timestamp:   ch.ConversationTimestamp,
	}
}

