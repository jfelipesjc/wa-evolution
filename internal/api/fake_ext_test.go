package api

import (
	"context"

	wa "github.com/jfelipesjc/wa-go/wa"
)

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

// --- communities ---

func (f *fakeBackend) CommunityCreate(ctx context.Context, name, subject, description string) (*wa.GroupInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityCreates = append(f.communityCreates, sentCommunityCreate{subject, description})
	if f.communityInfo != nil {
		return f.communityInfo, nil
	}
	return &wa.GroupInfo{JID: "120363000@g.us", Subject: subject, Desc: description}, nil
}

func (f *fakeBackend) CommunityMetadata(ctx context.Context, name, communityJID string) (*wa.GroupInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.communityInfo != nil {
		return f.communityInfo, nil
	}
	return &wa.GroupInfo{
		JID:     communityJID,
		Subject: "Community",
		Participants: []wa.GroupParticipant{
			{JID: "5512@s.whatsapp.net", IsAdmin: true, IsSuperAdmin: true},
		},
	}, nil
}

func (f *fakeBackend) CommunityUpdateSubject(ctx context.Context, name, communityJID, subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communitySubjects = append(f.communitySubjects, [2]string{communityJID, subject})
	return nil
}

func (f *fakeBackend) CommunityUpdateDescription(ctx context.Context, name, communityJID, description string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityDescs = append(f.communityDescs, [2]string{communityJID, description})
	return nil
}

func (f *fakeBackend) CommunityLinkGroups(ctx context.Context, name, communityJID string, groupJIDs []string) ([]CommunityLinkResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]CommunityLinkResult, 0, len(groupJIDs))
	for _, g := range groupJIDs {
		f.communityLinks = append(f.communityLinks, sentCommunityLink{communityJID, g, "link"})
		out = append(out, CommunityLinkResult{JID: g, Success: true})
	}
	return out, nil
}

func (f *fakeBackend) CommunityUnlinkGroup(ctx context.Context, name, communityJID, groupJID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityLinks = append(f.communityLinks, sentCommunityLink{communityJID, groupJID, "unlink"})
	return nil
}

func (f *fakeBackend) CommunityLinkedGroups(ctx context.Context, name, communityJID string) ([]wa.GroupLinkInfo, error) {
	return f.linkedGroups, nil
}

func (f *fakeBackend) CommunityRequestList(ctx context.Context, name, communityJID string) ([]wa.CommunityMembershipRequest, error) {
	return f.communityReqs, nil
}

func (f *fakeBackend) CommunityRequestUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityReqUpdates = append(f.communityReqUpdates, sentCommunityPart{communityJID, action, participants})
	out := make([]ParticipantResult, 0, len(participants))
	for _, p := range participants {
		out = append(out, ParticipantResult{JID: p, Status: "200"})
	}
	return out, nil
}

func (f *fakeBackend) CommunityParticipantsUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityPartUpdates = append(f.communityPartUpdates, sentCommunityPart{communityJID, action, participants})
	out := make([]ParticipantResult, 0, len(participants))
	for _, p := range participants {
		out = append(out, ParticipantResult{JID: p, Status: "200"})
	}
	return out, nil
}

func (f *fakeBackend) CommunityJoinApprovalMode(ctx context.Context, name, communityJID, mode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityModes = append(f.communityModes, sentCommunityMode{communityJID, "join", mode})
	return nil
}

func (f *fakeBackend) CommunityMemberAddMode(ctx context.Context, name, communityJID, mode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityModes = append(f.communityModes, sentCommunityMode{communityJID, "memberadd", mode})
	return nil
}

func (f *fakeBackend) CommunityToggleEphemeral(ctx context.Context, name, communityJID string, expiration int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityEphemerals = append(f.communityEphemerals, sentCommunityEphemeral{communityJID, expiration})
	return nil
}

func (f *fakeBackend) CommunitySettingUpdate(ctx context.Context, name, communityJID, setting string) error {
	f.mu.Lock()
	f.lastCommunitySetting = setting
	f.mu.Unlock()
	return nil
}

func (f *fakeBackend) CommunityLeave(ctx context.Context, name, communityJID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.communityLeaves = append(f.communityLeaves, communityJID)
	return nil
}

func (f *fakeBackend) CommunityFetchAllParticipating(ctx context.Context, name string) ([]wa.GroupLinkInfo, error) {
	return f.communityParticipating, nil
}

// --- newsletter admin ---

func (f *fakeBackend) newsletterInfoOr(jid string) *wa.NewsletterInfo {
	if f.newsletterInfo != nil {
		return f.newsletterInfo
	}
	return &wa.NewsletterInfo{JID: jid, Name: "Channel", SubscriberCount: 7}
}

func (f *fakeBackend) NewsletterMetadata(ctx context.Context, name, key, keyType string) (*wa.NewsletterInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.newsletterInfoOr(key), nil
}

func (f *fakeBackend) NewsletterUnfollow(ctx context.Context, name, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterUnfollows = append(f.newsletterUnfollows, jid)
	return nil
}

func (f *fakeBackend) NewsletterMute(ctx context.Context, name, jid string, mute bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterMutes = append(f.newsletterMutes, sentNewsletterMute{jid, mute})
	return nil
}

func (f *fakeBackend) NewsletterUpdateName(ctx context.Context, name, jid, newName string) (*wa.NewsletterInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterUpdates = append(f.newsletterUpdates, sentNewsletterUpdate{jid, "name", newName})
	return f.newsletterInfoOr(jid), nil
}

func (f *fakeBackend) NewsletterUpdateDescription(ctx context.Context, name, jid, desc string) (*wa.NewsletterInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterUpdates = append(f.newsletterUpdates, sentNewsletterUpdate{jid, "description", desc})
	return f.newsletterInfoOr(jid), nil
}

func (f *fakeBackend) NewsletterUpdatePicture(ctx context.Context, name, jid, picture string) (*wa.NewsletterInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterUpdates = append(f.newsletterUpdates, sentNewsletterUpdate{jid, "picture", picture})
	return f.newsletterInfoOr(jid), nil
}

func (f *fakeBackend) NewsletterReactionMode(ctx context.Context, name, jid, mode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastNewsletterReactionMode = mode
	return nil
}

func (f *fakeBackend) NewsletterFetchMessages(ctx context.Context, name, jid string, count int, since int64) ([]wa.NewsletterMessage, error) {
	f.mu.Lock()
	f.newsletterFetches = append(f.newsletterFetches, sentNewsletterFetch{jid, count, since})
	f.mu.Unlock()
	return f.newsletterMsgs, nil
}

func (f *fakeBackend) NewsletterAdminCount(ctx context.Context, name, jid string) (int, error) {
	return f.newsletterAdminCnt, nil
}

func (f *fakeBackend) NewsletterChangeOwner(ctx context.Context, name, jid, newOwnerJid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterOwnerChanges = append(f.newsletterOwnerChanges, sentNewsletterUser{jid, newOwnerJid})
	return nil
}

func (f *fakeBackend) NewsletterDemote(ctx context.Context, name, jid, userJid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterDemotes = append(f.newsletterDemotes, sentNewsletterUser{jid, userJid})
	return nil
}

func (f *fakeBackend) NewsletterSubscribeLiveUpdates(ctx context.Context, name, jid string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterSubscribes = append(f.newsletterSubscribes, jid)
	return f.newsletterSubDuration, nil
}

func (f *fakeBackend) NewsletterDelete(ctx context.Context, name, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterDeletes = append(f.newsletterDeletes, jid)
	return nil
}

func (f *fakeBackend) NewsletterSubscriberCount(ctx context.Context, name, jid string) (int, error) {
	return f.newsletterSubCount, nil
}

func (f *fakeBackend) NewsletterReactMessage(ctx context.Context, name, jid, serverID, reaction string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.newsletterReacts = append(f.newsletterReacts, sentNewsletterReact{jid, serverID, reaction})
	return nil
}
