package api

import "context"

// fakeBackend implementations for the extended Evolution-parity surface. They
// record just enough to let the handler tests assert routing, validation and
// response shaping offline.

func (f *fakeBackend) Restart(name string) error {
	if !f.Exists(name) {
		return ErrInstanceNotFound
	}
	return nil
}

func (f *fakeBackend) SendPoll(ctx context.Context, name, jid, pollName string, options []string, selectableCount int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls = append(f.polls, sentPoll{name, jid, pollName, options, selectableCount})
	return "MSGID-POLL", nil
}

func (f *fakeBackend) SendSticker(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error) {
	return "MSGID-STICKER", nil
}

func (f *fakeBackend) SendWhatsAppAudio(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error) {
	return "MSGID-AUDIO", nil
}

func (f *fakeBackend) SendPtv(ctx context.Context, name, jid string, data []byte, mimetype string) (string, error) {
	return "MSGID-PTV", nil
}

func (f *fakeBackend) ArchiveChat(ctx context.Context, name, jid string, archive bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.archives = append(f.archives, sentArchive{name, jid, archive})
	return nil
}

func (f *fakeBackend) FetchProfilePictureURL(ctx context.Context, name, jid string, preview bool) (string, error) {
	return "https://pic/" + jid, nil
}

func (f *fakeBackend) FetchBusinessProfile(ctx context.Context, name, jid string) (BusinessProfileArg, error) {
	return BusinessProfileArg{JID: jid, Description: "biz"}, nil
}

func (f *fakeBackend) FetchPrivacy(ctx context.Context, name string) (map[string]string, error) {
	return map[string]string{"lastSeen": "contacts"}, nil
}

func (f *fakeBackend) UpdatePrivacy(ctx context.Context, name, setting, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.privacy = append(f.privacy, [2]string{setting, value})
	return nil
}

func (f *fakeBackend) UpdateBlockStatus(ctx context.Context, name, jid string, block bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocks = append(f.blocks, sentBlock{jid, block})
	return nil
}

func (f *fakeBackend) UpdateProfilePicture(ctx context.Context, name, jid string, jpeg []byte) error {
	return nil
}

func (f *fakeBackend) RemoveProfilePicture(ctx context.Context, name, jid string) error {
	return nil
}

func (f *fakeBackend) FetchProfile(ctx context.Context, name, jid string) (ProfileArg, error) {
	return ProfileArg{JID: jid, PictureURL: "https://pic/" + jid, Status: "hey"}, nil
}

func (f *fakeBackend) FindChats(name string) ([]ChatInfoArg, error) {
	return f.chats, nil
}

func (f *fakeBackend) FindChatByRemoteJID(name, jid string) (ChatInfoArg, bool, error) {
	for _, ch := range f.chats {
		if ch.JID == jid {
			return ch, true, nil
		}
	}
	return ChatInfoArg{}, false, nil
}

func (f *fakeBackend) FindContacts(name string) ([]ContactArg, error) {
	return f.contactsList, nil
}

func (f *fakeBackend) GroupAcceptInvite(ctx context.Context, name, code string) (string, error) {
	return "123@g.us", nil
}

func (f *fakeBackend) GroupInviteInfo(ctx context.Context, name, code string) (GroupArg, error) {
	return GroupArg{JID: "123@g.us", Subject: "Invited Group"}, nil
}

func (f *fakeBackend) GroupRevokeInvite(ctx context.Context, name, groupJID string) (string, error) {
	return "NEWCODE", nil
}

func (f *fakeBackend) GroupToggleEphemeral(ctx context.Context, name, groupJID string, seconds int) error {
	return nil
}

func (f *fakeBackend) GroupUpdateDescription(ctx context.Context, name, groupJID, desc string) error {
	return nil
}

func (f *fakeBackend) GroupUpdateSubject(ctx context.Context, name, groupJID, subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupSubjects = append(f.groupSubjects, [2]string{groupJID, subject})
	return nil
}

func (f *fakeBackend) GroupUpdatePicture(ctx context.Context, name, groupJID string, jpeg []byte) error {
	return nil
}

func (f *fakeBackend) GroupUpdateSetting(ctx context.Context, name, groupJID, setting string) error {
	f.mu.Lock()
	f.lastGroupSetting = setting
	f.mu.Unlock()
	return nil
}

func (f *fakeBackend) GroupSendInvite(ctx context.Context, name, groupJID string, numbers []string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupInvites = append(f.groupInvites, sentGroupInvite{groupJID, numbers, text})
	return nil
}

func (f *fakeBackend) GetLabels(ctx context.Context, name string) ([]LabelArg, error) {
	return f.labels, nil
}

func (f *fakeBackend) HandleLabel(ctx context.Context, name, chatJID, labelID, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.labelOps = append(f.labelOps, sentLabelOp{chatJID, labelID, action})
	return nil
}

func (f *fakeBackend) OfferCall(ctx context.Context, name, jid string, video bool) (string, error) {
	return "CALLID-1", nil
}

func (f *fakeBackend) GetCatalog(ctx context.Context, name, jid string, limit int) ([]ProductArg, error) {
	return f.products, nil
}

func (f *fakeBackend) GetBase64FromMedia(ctx context.Context, name, jid, msgID string) ([]byte, string, error) {
	if f.mediaBytes == nil {
		return nil, "", ErrInstanceNotFound
	}
	return f.mediaBytes, f.mediaMime, nil
}

func (f *fakeBackend) MarkChatUnread(ctx context.Context, name, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unreads = append(f.unreads, jid)
	return nil
}

func (f *fakeBackend) PinChat(ctx context.Context, name, jid string, pin bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pins = append(f.pins, sentPin{jid, pin})
	return nil
}

func (f *fakeBackend) MuteChat(ctx context.Context, name, jid string, seconds int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mutes = append(f.mutes, sentMute{jid, seconds})
	return nil
}

func (f *fakeBackend) StarMessage(ctx context.Context, name, jid, msgID string, fromMe, star bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stars = append(f.stars, sentStar{jid, msgID, fromMe, star})
	return nil
}

func (f *fakeBackend) ClearChat(ctx context.Context, name, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears = append(f.clears, jid)
	return nil
}

func (f *fakeBackend) DeleteChat(ctx context.Context, name, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chatDeletes = append(f.chatDeletes, jid)
	return nil
}

func (f *fakeBackend) ResyncAppState(ctx context.Context, name string, collections []string, fresh bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resyncs = append(f.resyncs, sentResync{collections, fresh})
	return nil
}
