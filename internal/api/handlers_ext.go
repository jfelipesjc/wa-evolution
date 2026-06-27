package api

import (
	"encoding/base64"
	"net/http"
	"sync"
)

// --- instance ---

// handleRestart: PUT /instance/restart/{instance}.
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err := s.backend.Restart(name); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleSetPresence: POST /instance/setPresence/{instance} {presence}. Global
// presence (available|unavailable).
func (s *Server) handleSetPresence(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req setPresenceReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Presence {
	case "available", "unavailable":
	default:
		s.writeError(w, http.StatusBadRequest, "presence must be available|unavailable")
		return
	}
	if err := s.backend.SendPresence(r.Context(), name, "", req.Presence); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// --- message ---

// handleSendPoll: POST /message/sendPoll/{instance} {number, name, values[], selectableCount}.
func (s *Server) handleSendPoll(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendPollReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Name == "" || len(req.Values) < 2 {
		s.writeError(w, http.StatusBadRequest, "number, name and at least 2 values are required")
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendPoll(r.Context(), name, jid, req.Name, req.Values, req.SelectableCount)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{Key: messageKey{RemoteJID: jid, FromMe: true, ID: id}, Status: sendStatusPending})
}

// handleSendSticker: POST /message/sendSticker/{instance} {number, sticker, mimetype?}.
func (s *Server) handleSendSticker(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendStickerReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Sticker == "" {
		s.writeError(w, http.StatusBadRequest, "number and sticker are required")
		return
	}
	data, err := decodeMedia(req.Sticker)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "sticker must be base64: "+err.Error())
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendSticker(r.Context(), name, jid, data, req.Mimetype)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{Key: messageKey{RemoteJID: jid, FromMe: true, ID: id}, Status: sendStatusPending})
}

// handleSendWhatsAppAudio: POST /message/sendWhatsAppAudio/{instance} {number, audio, mimetype?}.
func (s *Server) handleSendWhatsAppAudio(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendAudioReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Audio == "" {
		s.writeError(w, http.StatusBadRequest, "number and audio are required")
		return
	}
	data, err := decodeMedia(req.Audio)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "audio must be base64: "+err.Error())
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendWhatsAppAudio(r.Context(), name, jid, data, req.Mimetype)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{Key: messageKey{RemoteJID: jid, FromMe: true, ID: id}, Status: sendStatusPending})
}

// handleSendPtv: POST /message/sendPtv/{instance} {number, video, mimetype?}.
func (s *Server) handleSendPtv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendPtvReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Video == "" {
		s.writeError(w, http.StatusBadRequest, "number and video are required")
		return
	}
	data, err := decodeMedia(req.Video)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "video must be base64: "+err.Error())
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendPtv(r.Context(), name, jid, data, req.Mimetype)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{Key: messageKey{RemoteJID: jid, FromMe: true, ID: id}, Status: sendStatusPending})
}

// --- chat ---

// handleArchiveChat: POST /chat/archiveChat/{instance} {chat, archive}.
func (s *Server) handleArchiveChat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req archiveChatReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Chat == "" {
		s.writeError(w, http.StatusBadRequest, "chat is required")
		return
	}
	jid := normalizeJID(req.Chat)
	if err := s.backend.ArchiveChat(r.Context(), name, jid, req.Archive); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleFetchProfilePicture: POST /chat/fetchProfilePictureUrl/{instance} {number}.
func (s *Server) handleFetchProfilePicture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req jidQueryReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	url, err := s.backend.FetchProfilePictureURL(r.Context(), name, jid, false)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, profilePictureResp{WUID: jid, ProfilePic: url})
}

// handleFetchBusinessProfile: POST /chat/fetchBusinessProfile/{instance} {number}.
func (s *Server) handleFetchBusinessProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req jidQueryReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	p, err := s.backend.FetchBusinessProfile(r.Context(), name, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, businessProfileResp{
		WUID: jid, Address: p.Address, Description: p.Description, Website: p.Website, Email: p.Email,
	})
}

// handleFetchProfile: POST /chat/fetchProfile/{instance} {number}.
func (s *Server) handleFetchProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req jidQueryReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	p, err := s.backend.FetchProfile(r.Context(), name, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, profileResp{WUID: jid, PictureURL: p.PictureURL, Status: p.Status})
}

// handleFetchPrivacy: GET /chat/fetchPrivacySettings/{instance}.
func (s *Server) handleFetchPrivacy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	settings, err := s.backend.FetchPrivacy(r.Context(), name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, settings)
}

// handleUpdatePrivacy: POST /chat/updatePrivacySettings/{instance}
// {name,value} or {settings:{name:value}}.
func (s *Server) handleUpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updatePrivacyReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	pairs := map[string]string{}
	if req.Name != "" {
		pairs[req.Name] = req.Value
	}
	for k, v := range req.Settings {
		pairs[k] = v
	}
	if len(pairs) == 0 {
		s.writeError(w, http.StatusBadRequest, "name+value or settings map is required")
		return
	}
	for k, v := range pairs {
		if err := s.backend.UpdatePrivacy(r.Context(), name, k, v); err != nil {
			s.writeSendError(w, err)
			return
		}
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleUpdateBlockStatus: POST /chat/updateBlockStatus/{instance} {number, status}.
func (s *Server) handleUpdateBlockStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateBlockStatusReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	var block bool
	switch req.Status {
	case "block":
		block = true
	case "unblock":
		block = false
	default:
		s.writeError(w, http.StatusBadRequest, "status must be block|unblock")
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	if err := s.backend.UpdateBlockStatus(r.Context(), name, jid, block); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleUpdateProfilePicture: PUT /chat/updateProfilePicture/{instance} {picture, number?}.
func (s *Server) handleUpdateProfilePicture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateProfilePictureReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Picture == "" {
		s.writeError(w, http.StatusBadRequest, "picture is required")
		return
	}
	data, err := decodeMedia(req.Picture)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "picture must be base64: "+err.Error())
		return
	}
	jid := ""
	if req.Number != "" {
		jid = normalizeJID(req.Number)
	}
	if err := s.backend.UpdateProfilePicture(r.Context(), name, jid, data); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleRemoveProfilePicture: DELETE /chat/removeProfilePicture/{instance}?number=.
func (s *Server) handleRemoveProfilePicture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	jid := ""
	if num := r.URL.Query().Get("number"); num != "" {
		jid = normalizeJID(num)
	}
	if err := s.backend.RemoveProfilePicture(r.Context(), name, jid); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleFindChats: GET|POST /chat/findChats/{instance}.
func (s *Server) handleFindChats(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	chats, err := s.backend.FindChats(name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]chatRecord, 0, len(chats))
	for _, ch := range chats {
		out = append(out, chatToRecord(ch))
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleFindChatByRemoteJid: POST /chat/findChatByRemoteJid/{instance} {number}.
func (s *Server) handleFindChatByRemoteJid(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req jidQueryReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	ch, ok, err := s.backend.FindChatByRemoteJID(name, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	s.writeJSON(w, http.StatusOK, chatToRecord(ch))
}

// handleFindContacts: GET|POST /chat/findContacts/{instance}.
func (s *Server) handleFindContacts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	contacts, err := s.backend.FindContacts(name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]contactRecord, 0, len(contacts))
	for _, ct := range contacts {
		out = append(out, contactRecord{ID: ct.JID, Name: ct.Name, Notify: ct.Notify, PushName: ct.PushName})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// --- group ---

// handleGroupParticipants: GET /group/participants/{instance}?groupJid=.
func (s *Server) handleGroupParticipants(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	jid := r.URL.Query().Get("groupJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid query param is required")
		return
	}
	g, err := s.backend.GroupMetadata(r.Context(), name, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]groupParticipant, 0, len(g.Participants))
	for _, p := range g.Participants {
		out = append(out, groupParticipant{ID: p.JID, Admin: p.Admin})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"participants": out})
}

// handleGroupInviteInfo: GET /group/inviteInfo/{instance}?inviteCode=.
func (s *Server) handleGroupInviteInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	code := r.URL.Query().Get("inviteCode")
	if code == "" {
		s.writeError(w, http.StatusBadRequest, "inviteCode query param is required")
		return
	}
	g, err := s.backend.GroupInviteInfo(r.Context(), name, code)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, groupToRecord(g))
}

// handleGroupAcceptInvite: POST /group/acceptInviteCode/{instance} {inviteCode}.
func (s *Server) handleGroupAcceptInvite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req acceptInviteReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.InviteCode == "" {
		s.writeError(w, http.StatusBadRequest, "inviteCode is required")
		return
	}
	jid, err := s.backend.GroupAcceptInvite(r.Context(), name, req.InviteCode)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, acceptInviteResp{GroupJID: jid})
}

// handleGroupRevokeInvite: PUT /group/revokeInviteCode/{instance}?groupJid=.
func (s *Server) handleGroupRevokeInvite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	jid := r.URL.Query().Get("groupJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid query param is required")
		return
	}
	code, err := s.backend.GroupRevokeInvite(r.Context(), name, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, revokeInviteResp{InviteCode: code, InviteURL: "https://chat.whatsapp.com/" + code})
}

// handleGroupSendInvite: POST /group/sendInvite/{instance} {groupJid, numbers[], description?}.
func (s *Server) handleGroupSendInvite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendInviteReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.GroupJID == "" || len(req.Numbers) == 0 {
		s.writeError(w, http.StatusBadRequest, "groupJid and numbers are required")
		return
	}
	nums := make([]string, 0, len(req.Numbers))
	for _, n := range req.Numbers {
		nums = append(nums, normalizeJID(n))
	}
	if err := s.backend.GroupSendInvite(r.Context(), name, req.GroupJID, nums, req.Description); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleGroupUpdateSubject: PUT /group/updateGroupSubject/{instance} {groupJid, subject}.
func (s *Server) handleGroupUpdateSubject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateGroupSubjectReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.GroupJID == "" || req.Subject == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid and subject are required")
		return
	}
	if err := s.backend.GroupUpdateSubject(r.Context(), name, req.GroupJID, req.Subject); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleGroupUpdateDescription: PUT /group/updateGroupDescription/{instance} {groupJid, description}.
func (s *Server) handleGroupUpdateDescription(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateGroupDescriptionReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.GroupJID == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid is required")
		return
	}
	if err := s.backend.GroupUpdateDescription(r.Context(), name, req.GroupJID, req.Description); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleGroupUpdatePicture: PUT /group/updateGroupPicture/{instance} {groupJid, picture}.
func (s *Server) handleGroupUpdatePicture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateGroupPictureReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.GroupJID == "" || req.Picture == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid and picture are required")
		return
	}
	data, err := decodeMedia(req.Picture)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "picture must be base64: "+err.Error())
		return
	}
	if err := s.backend.GroupUpdatePicture(r.Context(), name, req.GroupJID, data); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleGroupToggleEphemeral: PUT /group/toggleEphemeral/{instance} {groupJid, expiration}.
func (s *Server) handleGroupToggleEphemeral(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req toggleEphemeralReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.GroupJID == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid is required")
		return
	}
	if err := s.backend.GroupToggleEphemeral(r.Context(), name, req.GroupJID, req.Expiration); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleGroupUpdateSetting: PUT /group/updateSetting/{instance} {groupJid, action}.
func (s *Server) handleGroupUpdateSetting(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req updateGroupSettingReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Action {
	case "announcement", "not_announcement", "locked", "unlocked":
	default:
		s.writeError(w, http.StatusBadRequest, "action must be announcement|not_announcement|locked|unlocked")
		return
	}
	if req.GroupJID == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid is required")
		return
	}
	// Evolution names are announcement/not_announcement; the wa-go lib expects
	// announce/not_announce (locked/unlocked match). Map before delegating.
	setting := req.Action
	switch setting {
	case "announcement":
		setting = "announce"
	case "not_announcement":
		setting = "not_announce"
	}
	if err := s.backend.GroupUpdateSetting(r.Context(), name, req.GroupJID, setting); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// --- labels ---

// handleFindLabels: GET /label/findLabels/{instance}.
func (s *Server) handleFindLabels(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	labels, err := s.backend.GetLabels(r.Context(), name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]labelRecord, 0, len(labels))
	for _, l := range labels {
		out = append(out, labelRecord{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleHandleLabel: POST /label/handleLabel/{instance} {number, labelId, action}.
func (s *Server) handleHandleLabel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req handleLabelReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Action {
	case "add", "remove":
	default:
		s.writeError(w, http.StatusBadRequest, "action must be add|remove")
		return
	}
	if req.Number == "" || req.LabelID == "" {
		s.writeError(w, http.StatusBadRequest, "number and labelId are required")
		return
	}
	jid := normalizeJID(req.Number)
	if err := s.backend.HandleLabel(r.Context(), name, jid, req.LabelID, req.Action); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// --- call ---

// handleOfferCall: POST /call/offer/{instance} {number, isVideo}.
func (s *Server) handleOfferCall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req offerCallReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.OfferCall(r.Context(), name, jid, req.IsVideo)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, offerCallResp{CallID: id, Status: "OFFER"})
}

// handleGetBase64FromMedia: POST /chat/getBase64FromMediaMessage/{instance}.
// Accepts {message:{key:{remoteJid,id}}}, {key:{...}} or {number,messageId}.
func (s *Server) handleGetBase64FromMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req getBase64Req
	if !s.decodeJSON(w, r, &req) {
		return
	}
	var rawJID, msgID string
	switch {
	case req.Message != nil && req.Message.Key.ID != "":
		rawJID, msgID = req.Message.Key.RemoteJID, req.Message.Key.ID
	case req.Key != nil && req.Key.ID != "":
		rawJID, msgID = req.Key.RemoteJID, req.Key.ID
	default:
		rawJID, msgID = req.Number, req.MessageID
	}
	if rawJID == "" || msgID == "" {
		s.writeError(w, http.StatusBadRequest, "message.key{remoteJid,id} or {number,messageId} is required")
		return
	}
	jid := normalizeJID(rawJID)
	data, mime, err := s.backend.GetBase64FromMedia(r.Context(), name, jid, msgID)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, getBase64Resp{Mimetype: mime, Base64: base64.StdEncoding.EncodeToString(data)})
}

// handleMarkChatUnread: POST /chat/markChatUnread/{instance} {chat|number}.
func (s *Server) handleMarkChatUnread(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req markChatUnreadReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	raw := req.Chat
	if raw == "" {
		raw = req.Number
	}
	if raw == "" {
		s.writeError(w, http.StatusBadRequest, "chat or number is required")
		return
	}
	jid := normalizeJID(raw)
	if err := s.backend.MarkChatUnread(r.Context(), name, jid); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handlePinChat: POST /chat/pinChat/{instance} {chat, pin}.
func (s *Server) handlePinChat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req pinChatReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Chat == "" {
		s.writeError(w, http.StatusBadRequest, "chat is required")
		return
	}
	jid := normalizeJID(req.Chat)
	if err := s.backend.PinChat(r.Context(), name, jid, req.Pin); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleMuteChat: POST /chat/muteChat/{instance} {chat, duration}. Duration is
// in seconds (0 = unmute).
func (s *Server) handleMuteChat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req muteChatReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Chat == "" {
		s.writeError(w, http.StatusBadRequest, "chat is required")
		return
	}
	jid := normalizeJID(req.Chat)
	if err := s.backend.MuteChat(r.Context(), name, jid, req.Duration); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleStarMessage: POST /message/starMessage/{instance}. Accepts
// {key:{remoteJid,id,fromMe}} and a {number, messageId, fromMe} form, plus star.
func (s *Server) handleStarMessage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req starMessageReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	var rawJID, msgID string
	var fromMe bool
	if req.Key != nil && req.Key.ID != "" {
		rawJID, msgID, fromMe = req.Key.RemoteJID, req.Key.ID, req.Key.FromMe
	} else {
		rawJID, msgID, fromMe = req.Number, req.MessageID, req.FromMe
	}
	if rawJID == "" || msgID == "" {
		s.writeError(w, http.StatusBadRequest, "key{remoteJid,id} or {number,messageId} is required")
		return
	}
	jid := normalizeJID(rawJID)
	if err := s.backend.StarMessage(r.Context(), name, jid, msgID, fromMe, req.Star); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleClearChat: POST /chat/clearChat/{instance} {chat}.
func (s *Server) handleClearChat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req clearChatReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Chat == "" {
		s.writeError(w, http.StatusBadRequest, "chat is required")
		return
	}
	jid := normalizeJID(req.Chat)
	if err := s.backend.ClearChat(r.Context(), name, jid); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleDeleteChat: POST /chat/deleteChat/{instance} {chat}.
func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req deleteChatReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Chat == "" {
		s.writeError(w, http.StatusBadRequest, "chat is required")
		return
	}
	jid := normalizeJID(req.Chat)
	if err := s.backend.DeleteChat(r.Context(), name, jid); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleResyncAppState: POST /chat/resyncAppState/{instance} {collections, fresh}.
func (s *Server) handleResyncAppState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req resyncAppStateReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if len(req.Collections) == 0 {
		s.writeError(w, http.StatusBadRequest, "collections is required")
		return
	}
	if err := s.backend.ResyncAppState(r.Context(), name, req.Collections, req.Fresh); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleFindStatusMessage: POST /chat/findStatusMessage/{instance}. Returns the
// stored status@broadcast (stories) messages, reusing the findMessages store.
func (s *Server) handleFindStatusMessage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	// limit is optional; 0 = all stored.
	var req findMessagesReq
	_ = s.tryDecodeJSON(r, &req)
	msgs, err := s.backend.FindMessages(name, "status@broadcast", req.Limit)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	var resp findMessagesResp
	resp.Messages.Records = make([]messageRecord, 0, len(msgs))
	for _, m := range msgs {
		rec := messageRecord{
			Key:              messageKey{RemoteJID: m.ChatJID, FromMe: m.FromMe, ID: m.ID},
			MessageType:      m.Type,
			MessageTimestamp: m.Timestamp,
		}
		rec.Message.Conversation = m.Text
		resp.Messages.Records = append(resp.Messages.Records, rec)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// --- business ---

// handleGetCollections: POST /business/getCollections/{instance} {number, limit}.
// WhatsApp catalog collections are not separately fetched by the library; this
// returns the full catalog wrapped as a single "All products" collection so the
// route is functional and shape-compatible.
func (s *Server) handleGetCollections(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req getCatalogReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	products, err := s.backend.GetCatalog(r.Context(), name, jid, req.Limit)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	recs := make([]productRecord, 0, len(products))
	for _, p := range products {
		recs = append(recs, productRecord{ID: p.ID, Name: p.Name, Description: p.Description, Price: p.Price, Currency: p.Currency})
	}
	s.writeJSON(w, http.StatusOK, []collectionRecord{{ID: "all", Name: "All products", Products: recs}})
}

// handleGetCatalog: POST /business/getCatalog/{instance} {number, limit}.
func (s *Server) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req getCatalogReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" {
		s.writeError(w, http.StatusBadRequest, "number is required")
		return
	}
	jid := normalizeJID(req.Number)
	products, err := s.backend.GetCatalog(r.Context(), name, jid, req.Limit)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]productRecord, 0, len(products))
	for _, p := range products {
		out = append(out, productRecord{ID: p.ID, Name: p.Name, Description: p.Description, Price: p.Price, Currency: p.Currency})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// --- settings / proxy (instance config; stored + echoed) ---

// handleSetSettings: POST /settings/set/{instance}.
func (s *Server) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var req settingsBody
	if !s.decodeJSON(w, r, &req) {
		return
	}
	s.cfg.setSettings(name, req)
	s.writeJSON(w, http.StatusOK, map[string]any{"settings": req})
}

// handleFindSettings: GET /settings/find/{instance}.
func (s *Server) handleFindSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	s.writeJSON(w, http.StatusOK, s.cfg.settings(name))
}

// handleSetProxy: POST /proxy/set/{instance}.
func (s *Server) handleSetProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var req proxyBody
	if !s.decodeJSON(w, r, &req) {
		return
	}
	s.cfg.setProxy(name, req)
	s.writeJSON(w, http.StatusOK, map[string]any{"proxy": req})
}

// handleFindProxy: GET /proxy/find/{instance}.
func (s *Server) handleFindProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	s.writeJSON(w, http.StatusOK, s.cfg.proxy(name))
}

// --- helpers ---

func chatToRecord(ch ChatInfoArg) chatRecord {
	return chatRecord{
		ID:          ch.JID,
		Name:        ch.Name,
		UnreadCount: ch.UnreadCount,
		Archived:    ch.Archived,
		Pinned:      ch.Pinned,
		Muted:       ch.Muted,
		Timestamp:   ch.Timestamp,
	}
}

// configStore keeps per-instance settings/proxy blobs in memory.
type configStore struct {
	mu             sync.Mutex
	settingsByName map[string]settingsBody
	proxiesByName  map[string]proxyBody
}

func newConfigStore() *configStore {
	return &configStore{settingsByName: map[string]settingsBody{}, proxiesByName: map[string]proxyBody{}}
}

func (c *configStore) setSettings(name string, v settingsBody) {
	c.mu.Lock()
	c.settingsByName[name] = v
	c.mu.Unlock()
}

func (c *configStore) settings(name string) settingsBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settingsByName[name]
}

func (c *configStore) setProxy(name string, v proxyBody) {
	c.mu.Lock()
	c.proxiesByName[name] = v
	c.mu.Unlock()
}

func (c *configStore) proxy(name string) proxyBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.proxiesByName[name]
}
