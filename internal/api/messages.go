package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

// sendStatusPending is the Evolution status echoed for an accepted send (the
// message is on the wire; delivery/read come later via webhook).
const sendStatusPending = "PENDING"

// handleSendText: POST /message/sendText/{instance} {number, text}.
func (s *Server) handleSendText(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendTextReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Text == "" {
		s.writeError(w, http.StatusBadRequest, "number and text are required")
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendText(r.Context(), name, jid, req.Text)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{
		Key:    messageKey{RemoteJID: jid, FromMe: true, ID: id},
		Status: sendStatusPending,
	})
}

// handleSendMedia: POST /message/sendMedia/{instance}.
func (s *Server) handleSendMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendMediaReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Number == "" || req.Media == "" {
		s.writeError(w, http.StatusBadRequest, "number and media are required")
		return
	}
	kind := strings.ToLower(req.MediaType)
	switch kind {
	case "image", "video", "audio", "document":
	default:
		s.writeError(w, http.StatusBadRequest, "mediatype must be image|video|audio|document")
		return
	}
	data, err := decodeMedia(req.Media)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "media must be base64 (URL fetch not supported offline): "+err.Error())
		return
	}
	jid := normalizeJID(req.Number)
	id, err := s.backend.SendMedia(r.Context(), name, jid, MediaArg{
		Kind:     kind,
		Data:     data,
		Caption:  req.Caption,
		FileName: req.FileName,
		Mimetype: req.Mimetype,
	})
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{
		Key:    messageKey{RemoteJID: jid, FromMe: true, ID: id},
		Status: sendStatusPending,
	})
}

// handleSendReaction: POST /message/sendReaction/{instance} {key, reaction}.
func (s *Server) handleSendReaction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req sendReactionReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Key.RemoteJID == "" || req.Key.ID == "" {
		s.writeError(w, http.StatusBadRequest, "key.remoteJid and key.id are required")
		return
	}
	jid := normalizeJID(req.Key.RemoteJID)
	id, err := s.backend.SendReaction(r.Context(), name, jid, req.Key.ID, req.Key.FromMe, req.Reaction)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sendResp{
		Key:    messageKey{RemoteJID: jid, FromMe: true, ID: id},
		Status: sendStatusPending,
	})
}

// handleFindMessages: POST /chat/findMessages/{instance} {where:{key:{remoteJid}}, limit?}.
func (s *Server) handleFindMessages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req findMessagesReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := normalizeJID(req.Where.Key.RemoteJID)
	msgs, err := s.backend.FindMessages(name, jid, req.Limit)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			s.writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
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

// handleWhatsAppNumbers: POST /chat/whatsappNumbers/{instance} {numbers:[...]}.
func (s *Server) handleWhatsAppNumbers(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	var req whatsappNumbersReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if len(req.Numbers) == 0 {
		s.writeError(w, http.StatusBadRequest, "numbers is required")
		return
	}
	res, err := s.backend.WhatsAppNumbers(r.Context(), name, req.Numbers)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]numberStatus, 0, len(res))
	for _, n := range res {
		out = append(out, numberStatus{Exists: n.Exists, JID: n.JID, Number: n.Number})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleFetchAllGroups: GET /group/fetchAllGroups/{instance}.
func (s *Server) handleFetchAllGroups(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	groups, err := s.backend.Groups(r.Context(), name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]groupRecord, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupToRecord(g))
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleGroupMetadata: GET /group/groupMetadata/{instance}?groupJid=.
func (s *Server) handleGroupMetadata(w http.ResponseWriter, r *http.Request) {
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
	s.writeJSON(w, http.StatusOK, groupToRecord(g))
}

func groupToRecord(g GroupArg) groupRecord {
	rec := groupRecord{
		ID:       g.JID,
		Subject:  g.Subject,
		Owner:    g.Owner,
		Desc:     g.Desc,
		Creation: g.Creation,
	}
	for _, p := range g.Participants {
		rec.Participants = append(rec.Participants, groupParticipant{ID: p.JID, Admin: p.Admin})
	}
	return rec
}

// writeSendError maps backend errors to HTTP status codes.
func (s *Server) writeSendError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInstanceNotFound):
		s.writeError(w, http.StatusNotFound, "instance not found")
	case errors.Is(err, ErrNoSession):
		s.writeError(w, http.StatusConflict, "instance has no active session")
	default:
		s.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// decodeMedia decodes a base64 media payload, accepting both a raw base64 string
// and a data URI (data:<mime>;base64,<payload>). URL inputs are rejected (this
// build does not fetch remote media).
func decodeMedia(media string) ([]byte, error) {
	if i := strings.Index(media, ";base64,"); i >= 0 {
		media = media[i+len(";base64,"):]
	} else if strings.HasPrefix(media, "http://") || strings.HasPrefix(media, "https://") {
		return nil, errors.New("remote media URLs are not supported")
	}
	media = strings.TrimSpace(media)
	data, err := base64.StdEncoding.DecodeString(media)
	if err != nil {
		// Try raw-URL-safe encoding as a fallback.
		if d2, e2 := base64.RawStdEncoding.DecodeString(media); e2 == nil {
			return d2, nil
		}
		return nil, err
	}
	return data, nil
}
