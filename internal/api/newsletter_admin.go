package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	wa "github.com/jfelipesjc/wa-go/wa"
)

// Newsletter admin endpoints surface the wa-go channel (newsletter) admin and
// metadata features to HTTP callers, mirroring the Evolution API shape. Handlers
// + ManagerBackend implementations live together here; the Backend interface
// (backend.go), routes (server.go) and the test fake (fake_ext_test.go) are the
// shared edit points. /newsletter/create and /newsletter/follow already exist in
// features.go and are NOT re-declared here.
//
// JID handling: newsletter JIDs end in @newsletter and already carry their
// domain, so normalizeJID is a no-op for them; it only completes a bare phone
// number into <num>@s.whatsapp.net (used for userJid / owner targets).

// --- request shapes (Evolution-compatible field names) ---

type newsletterJidReq struct {
	NewsletterJid string `json:"newsletterJid"`
	JID           string `json:"jid"`
}

type newsletterUpdateNameReq struct {
	NewsletterJid string `json:"newsletterJid"`
	Name          string `json:"name"`
}

type newsletterUpdateDescriptionReq struct {
	NewsletterJid string `json:"newsletterJid"`
	Description   string `json:"description"`
}

type newsletterUpdatePictureReq struct {
	NewsletterJid string `json:"newsletterJid"`
	Picture       string `json:"picture"`
}

type newsletterReactionModeReq struct {
	NewsletterJid string `json:"newsletterJid"`
	Mode          string `json:"mode"`
}

type newsletterUserReq struct {
	NewsletterJid string `json:"newsletterJid"`
	UserJid       string `json:"userJid"`
}

// --- response shapes (camelCase, mapped from wa.* types) ---

type newsletterResp struct {
	JID             string `json:"jid"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Invite          string `json:"invite,omitempty"`
	SubscriberCount int    `json:"subscriberCount"`
	Verification    string `json:"verification,omitempty"`
	CreationTime    int64  `json:"creationTime,omitempty"`
	MuteState       string `json:"muteState,omitempty"`
}

type newsletterMessageRecord struct {
	ServerID  string `json:"serverId"`
	Timestamp int64  `json:"timestamp"`
	Type      string `json:"type,omitempty"`
	Content   string `json:"content,omitempty"` // base64 of the raw message bytes
}

// newsletterToResp maps the library NewsletterInfo into the HTTP response.
func newsletterToResp(info *wa.NewsletterInfo) newsletterResp {
	out := newsletterResp{}
	if info == nil {
		return out
	}
	out.JID = info.JID
	out.Name = info.Name
	out.Description = info.Description
	out.Invite = info.Invite
	out.SubscriberCount = info.SubscriberCount
	out.Verification = info.Verification
	out.CreationTime = info.CreationTime
	out.MuteState = info.MuteState
	return out
}

// newsletterJidFrom returns the newsletter JID from the body fields
// (newsletterJid preferred, then jid), falling back to the query params (the
// documented POST convention).
func newsletterJidFrom(r *http.Request, fromBody, fromBodyAlt string) string {
	if fromBody != "" {
		return fromBody
	}
	if fromBodyAlt != "" {
		return fromBodyAlt
	}
	if v := r.URL.Query().Get("newsletterJid"); v != "" {
		return v
	}
	return r.URL.Query().Get("jid")
}

// validReactionMode reports whether m (already upper-cased) is an accepted
// channel reaction policy.
func validReactionMode(m string) bool {
	switch m {
	case "ALL", "BASIC", "NONE", "BLOCKLIST":
		return true
	default:
		return false
	}
}

// --- HTTP handlers ---

// handleNewsletterFind: GET /newsletter/findNewsletter/{instance}?key=&type=jid|invite.
func (s *Server) handleNewsletterFind(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	key := r.URL.Query().Get("key")
	if key == "" {
		s.writeError(w, http.StatusBadRequest, "key query param is required")
		return
	}
	// Canonize the key type once (case-insensitive) so type=INVITE/Invite work.
	keyType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if keyType == "" {
		keyType = "jid"
	}
	// For a JID lookup, normalize (no-op for @newsletter); leave invite codes intact.
	if keyType != "invite" {
		key = normalizeJID(key)
	}
	info, err := s.backend.NewsletterMetadata(r.Context(), inst, key, keyType)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newsletterToResp(info))
}

// handleNewsletterUnfollow: POST /newsletter/unfollow/{instance} {jid|newsletterJid}.
func (s *Server) handleNewsletterUnfollow(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	// JID-only body: tolerate an empty body so the JID may arrive via query param.
	var req newsletterJidReq
	s.tryDecodeJSON(r, &req)
	jid := newsletterJidFrom(r, req.NewsletterJid, req.JID)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if err := s.backend.NewsletterUnfollow(r.Context(), inst, normalizeJID(jid)); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleNewsletterMute: POST /newsletter/mute/{instance} {newsletterJid|jid}.
func (s *Server) handleNewsletterMute(w http.ResponseWriter, r *http.Request) {
	s.newsletterSetMute(w, r, true)
}

// handleNewsletterUnmute: POST /newsletter/unmute/{instance} {newsletterJid|jid}.
func (s *Server) handleNewsletterUnmute(w http.ResponseWriter, r *http.Request) {
	s.newsletterSetMute(w, r, false)
}

func (s *Server) newsletterSetMute(w http.ResponseWriter, r *http.Request, mute bool) {
	inst := r.PathValue("instance")
	// JID-only body: tolerate an empty body so the JID may arrive via query param.
	var req newsletterJidReq
	s.tryDecodeJSON(r, &req)
	jid := newsletterJidFrom(r, req.NewsletterJid, req.JID)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if err := s.backend.NewsletterMute(r.Context(), inst, normalizeJID(jid), mute); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleNewsletterUpdateName: POST /newsletter/updateName/{instance} {newsletterJid, name}.
func (s *Server) handleNewsletterUpdateName(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterUpdateNameReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	info, err := s.backend.NewsletterUpdateName(r.Context(), inst, normalizeJID(jid), req.Name)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newsletterToResp(info))
}

// handleNewsletterUpdateDescription: POST /newsletter/updateDescription/{instance} {newsletterJid, description}.
func (s *Server) handleNewsletterUpdateDescription(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterUpdateDescriptionReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if req.Description == "" {
		s.writeError(w, http.StatusBadRequest, "description is required")
		return
	}
	info, err := s.backend.NewsletterUpdateDescription(r.Context(), inst, normalizeJID(jid), req.Description)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newsletterToResp(info))
}

// handleNewsletterUpdatePicture: POST /newsletter/updatePicture/{instance}
// {newsletterJid, picture}. picture is a base64 jpeg; an empty picture removes
// the channel photo (mapped to wa.NewsletterUpdateInput.Picture=="").
func (s *Server) handleNewsletterUpdatePicture(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterUpdatePictureReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	// picture=="" is intentional: it removes the channel photo.
	info, err := s.backend.NewsletterUpdatePicture(r.Context(), inst, normalizeJID(jid), req.Picture)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newsletterToResp(info))
}

// handleNewsletterReactionMode: POST /newsletter/reactionMode/{instance}
// {newsletterJid, mode ALL|BASIC|NONE|BLOCKLIST}.
func (s *Server) handleNewsletterReactionMode(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterReactionModeReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	mode := strings.ToUpper(strings.TrimSpace(req.Mode))
	if !validReactionMode(mode) {
		s.writeError(w, http.StatusBadRequest, "mode must be ALL|BASIC|NONE|BLOCKLIST")
		return
	}
	if err := s.backend.NewsletterReactionMode(r.Context(), inst, normalizeJID(jid), mode); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleNewsletterFetchMessages: GET /newsletter/fetchMessages/{instance}?newsletterJid=&count=&since=.
func (s *Server) handleNewsletterFetchMessages(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := newsletterJidFrom(r, "", "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid query param is required")
		return
	}
	count := 50
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
		}
	}
	var since int64
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	msgs, err := s.backend.NewsletterFetchMessages(r.Context(), inst, normalizeJID(jid), count, since)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]newsletterMessageRecord, 0, len(msgs))
	for _, m := range msgs {
		rec := newsletterMessageRecord{ServerID: m.ServerID, Timestamp: m.Timestamp, Type: m.Type}
		if len(m.Content) > 0 {
			rec.Content = base64.StdEncoding.EncodeToString(m.Content)
		}
		out = append(out, rec)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleNewsletterAdminCount: GET /newsletter/adminCount/{instance}?newsletterJid=.
func (s *Server) handleNewsletterAdminCount(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := newsletterJidFrom(r, "", "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid query param is required")
		return
	}
	n, err := s.backend.NewsletterAdminCount(r.Context(), inst, normalizeJID(jid))
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]int{"adminCount": n})
}

// handleNewsletterChangeOwner: POST /newsletter/changeOwner/{instance} {newsletterJid, userJid}.
func (s *Server) handleNewsletterChangeOwner(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterUserReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if req.UserJid == "" {
		s.writeError(w, http.StatusBadRequest, "userJid is required")
		return
	}
	if err := s.backend.NewsletterChangeOwner(r.Context(), inst, normalizeJID(jid), normalizeJID(req.UserJid)); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleNewsletterDemote: POST /newsletter/demote/{instance} {newsletterJid, userJid}.
func (s *Server) handleNewsletterDemote(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req newsletterUserReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := newsletterJidFrom(r, req.NewsletterJid, "")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	if req.UserJid == "" {
		s.writeError(w, http.StatusBadRequest, "userJid is required")
		return
	}
	if err := s.backend.NewsletterDemote(r.Context(), inst, normalizeJID(jid), normalizeJID(req.UserJid)); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleNewsletterSubscribe: POST /newsletter/subscribeUpdates/{instance} {newsletterJid}.
func (s *Server) handleNewsletterSubscribe(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	// JID-only body: tolerate an empty body so the JID may arrive via query param.
	var req newsletterJidReq
	s.tryDecodeJSON(r, &req)
	jid := newsletterJidFrom(r, req.NewsletterJid, req.JID)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "newsletterJid is required")
		return
	}
	dur, err := s.backend.NewsletterSubscribeLiveUpdates(r.Context(), inst, normalizeJID(jid))
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"duration": dur})
}

// --- ManagerBackend implementations ---

func (b *ManagerBackend) NewsletterMetadata(ctx context.Context, name, key, keyType string) (*wa.NewsletterInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	kt := wa.NewsletterKeyJID
	if strings.ToLower(strings.TrimSpace(keyType)) == "invite" {
		kt = wa.NewsletterKeyInvite
	}
	return c.NewsletterMetadata(ctx, key, kt)
}

func (b *ManagerBackend) NewsletterUnfollow(ctx context.Context, name, jid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterUnfollow(ctx, jid)
}

func (b *ManagerBackend) NewsletterMute(ctx context.Context, name, jid string, mute bool) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterMute(ctx, jid, mute)
}

func (b *ManagerBackend) NewsletterUpdateName(ctx context.Context, name, jid, newName string) (*wa.NewsletterInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	n := newName
	return c.NewsletterUpdate(ctx, jid, wa.NewsletterUpdateInput{Name: &n})
}

func (b *ManagerBackend) NewsletterUpdateDescription(ctx context.Context, name, jid, desc string) (*wa.NewsletterInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	d := desc
	return c.NewsletterUpdate(ctx, jid, wa.NewsletterUpdateInput{Description: &d})
}

func (b *ManagerBackend) NewsletterUpdatePicture(ctx context.Context, name, jid, picture string) (*wa.NewsletterInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	p := picture
	return c.NewsletterUpdate(ctx, jid, wa.NewsletterUpdateInput{Picture: &p})
}

func (b *ManagerBackend) NewsletterReactionMode(ctx context.Context, name, jid, mode string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterReactionMode(ctx, jid, wa.NewsletterReactionMode(strings.ToUpper(mode)))
}

func (b *ManagerBackend) NewsletterFetchMessages(ctx context.Context, name, jid string, count int, since int64) ([]wa.NewsletterMessage, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.NewsletterFetchMessages(ctx, jid, count, since)
}

func (b *ManagerBackend) NewsletterAdminCount(ctx context.Context, name, jid string) (int, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return 0, err
	}
	return c.NewsletterAdminCount(ctx, jid)
}

func (b *ManagerBackend) NewsletterChangeOwner(ctx context.Context, name, jid, newOwnerJid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterChangeOwner(ctx, jid, newOwnerJid)
}

func (b *ManagerBackend) NewsletterDemote(ctx context.Context, name, jid, userJid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterDemote(ctx, jid, userJid)
}

func (b *ManagerBackend) NewsletterSubscribeLiveUpdates(ctx context.Context, name, jid string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	sub, err := c.SubscribeLiveUpdates(ctx, jid)
	if err != nil {
		return "", err
	}
	if sub == nil {
		return "", nil
	}
	return sub.Duration, nil
}
