package api

import (
	"context"
	"net/http"

	wa "github.com/jfelipesjc/wa-go/wa"
)

// Community endpoints surface the wa-go community (announcement-group) features
// to HTTP callers, mirroring the Evolution API shape. Handlers + ManagerBackend
// implementations live together here; the Backend interface (backend.go), routes
// (server.go) and the test fake (fake_ext_test.go) are the shared edit points.
//
// JID handling: community JIDs end in @g.us and newsletter/group JIDs already
// carry their domain, so normalizeJID is a no-op for them; it only completes a
// bare phone number into <num>@s.whatsapp.net (used for participants).

// --- request shapes (Evolution-compatible field names) ---

type communityCreateReq struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
}

type communityUpdateSubjectReq struct {
	CommunityJid string `json:"communityJid"`
	Subject      string `json:"subject"`
}

type communityUpdateDescriptionReq struct {
	CommunityJid string `json:"communityJid"`
	Description  string `json:"description"`
}

type communityLinkGroupReq struct {
	CommunityJid string   `json:"communityJid"`
	Groups       []string `json:"groups"`
	GroupJid     string   `json:"groupJid"`
}

type communityUnlinkGroupReq struct {
	CommunityJid string `json:"communityJid"`
	GroupJid     string `json:"groupJid"`
}

type communityRequestUpdateReq struct {
	CommunityJid string   `json:"communityJid"`
	Participants []string `json:"participants"`
	Action       string   `json:"action"` // approve|reject
}

type communityParticipantReq struct {
	CommunityJid string   `json:"communityJid"`
	Participants []string `json:"participants"`
	Action       string   `json:"action"` // add|remove|promote|demote
}

type communityCreateGroupReq struct {
	CommunityJid string   `json:"communityJid"`
	Subject      string   `json:"subject"`
	Participants []string `json:"participants"`
}

type communityModeReq struct {
	CommunityJid string `json:"communityJid"`
	Mode         string `json:"mode"`
}

type communityToggleEphemeralReq struct {
	CommunityJid string `json:"communityJid"`
	Expiration   int    `json:"expiration"` // seconds (0,86400,604800,7776000)
}

type communitySettingReq struct {
	CommunityJid string `json:"communityJid"`
	Action       string `json:"action"` // announcement|not_announcement|locked|unlocked
}

// --- response shapes (camelCase, mapped from wa.* types) ---

type communityParticipant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"isAdmin"`
	IsSuperAdmin bool   `json:"isSuperAdmin"`
}

type communityResp struct {
	JID          string                 `json:"jid"`
	Subject      string                 `json:"subject"`
	Owner        string                 `json:"owner,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Creation     int64                  `json:"creation,omitempty"`
	Participants []communityParticipant `json:"participants"`
}

type communityLinkRecord struct {
	JID     string `json:"jid"`
	Subject string `json:"subject,omitempty"`
}

type communityRequestRecord struct {
	JID   string            `json:"jid"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

type communityLinkResultRecord struct {
	JID     string `json:"jid"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// CommunityLinkResult is the backend-neutral outcome of linking one group into a
// community (success + optional error message).
type CommunityLinkResult struct {
	JID     string
	Success bool
	Error   string
}

// communityToResp maps the library GroupInfo into the camelCase HTTP response.
func communityToResp(info *wa.GroupInfo) communityResp {
	out := communityResp{Participants: []communityParticipant{}}
	if info == nil {
		return out
	}
	out.JID = info.JID
	out.Subject = info.Subject
	out.Owner = info.Owner
	out.Description = info.Desc
	out.Creation = info.Creation
	for _, p := range info.Participants {
		out.Participants = append(out.Participants, communityParticipant{
			JID: p.JID, IsAdmin: p.IsAdmin, IsSuperAdmin: p.IsSuperAdmin,
		})
	}
	return out
}

// communityJidFrom returns the community JID from the body field, falling back to
// the communityJid query param (the documented POST convention).
func communityJidFrom(r *http.Request, fromBody string) string {
	if fromBody != "" {
		return fromBody
	}
	return r.URL.Query().Get("communityJid")
}

// --- HTTP handlers ---

// handleCommunityCreate: POST /community/create/{instance} {subject, description}.
func (s *Server) handleCommunityCreate(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityCreateReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Subject == "" {
		s.writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	info, err := s.backend.CommunityCreate(r.Context(), inst, req.Subject, req.Description)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, communityToResp(info))
}

// handleCommunityCreateGroup: POST /community/createGroup/{instance}
// {communityJid, subject, participants[]}. Creates a new sub-group under the
// community and returns its metadata (same shape as /community/create).
func (s *Server) handleCommunityCreateGroup(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityCreateGroupReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if req.Subject == "" {
		s.writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	parts := make([]string, 0, len(req.Participants))
	for _, p := range req.Participants {
		if nj := normalizeJID(p); nj != "" {
			parts = append(parts, nj)
		}
	}
	info, err := s.backend.CommunityCreateGroup(r.Context(), inst, normalizeJID(jid), req.Subject, parts)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, communityToResp(info))
}

// handleCommunityLinkedGroupsParticipants: GET
// /community/linkedGroupsParticipants/{instance}?communityJid=. Lists the JIDs of
// every participant across the community's linked sub-groups.
func (s *Server) handleCommunityLinkedGroupsParticipants(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := r.URL.Query().Get("communityJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid query param is required")
		return
	}
	parts, err := s.backend.CommunityLinkedGroupsParticipants(r.Context(), inst, normalizeJID(jid))
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	if parts == nil {
		parts = []string{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"participants": parts})
}

// handleCommunityFind: GET /community/findCommunity/{instance}?communityJid=.
func (s *Server) handleCommunityFind(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := r.URL.Query().Get("communityJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid query param is required")
		return
	}
	jid = normalizeJID(jid)
	info, err := s.backend.CommunityMetadata(r.Context(), inst, jid)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, communityToResp(info))
}

// handleCommunityUpdateSubject: POST /community/updateSubject/{instance} {communityJid, subject}.
func (s *Server) handleCommunityUpdateSubject(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityUpdateSubjectReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if req.Subject == "" {
		s.writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if err := s.backend.CommunityUpdateSubject(r.Context(), inst, normalizeJID(jid), req.Subject); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityUpdateDescription: POST /community/updateDescription/{instance} {communityJid, description}.
func (s *Server) handleCommunityUpdateDescription(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityUpdateDescriptionReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if err := s.backend.CommunityUpdateDescription(r.Context(), inst, normalizeJID(jid), req.Description); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityLinkGroup: POST /community/linkGroup/{instance}
// {communityJid, groups[] | groupJid}. Links each group, returning a per-group
// outcome under {"linked":[...]}.
func (s *Server) handleCommunityLinkGroup(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityLinkGroupReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	groups := req.Groups
	if len(groups) == 0 && req.GroupJid != "" {
		groups = []string{req.GroupJid}
	}
	if len(groups) == 0 {
		s.writeError(w, http.StatusBadRequest, "groups or groupJid is required")
		return
	}
	norm := make([]string, 0, len(groups))
	for _, g := range groups {
		if nj := normalizeJID(g); nj != "" {
			norm = append(norm, nj)
		}
	}
	if len(norm) == 0 {
		s.writeError(w, http.StatusBadRequest, "groups or groupJid is required")
		return
	}
	results, err := s.backend.CommunityLinkGroups(r.Context(), inst, normalizeJID(jid), norm)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]communityLinkResultRecord, 0, len(results))
	for _, res := range results {
		out = append(out, communityLinkResultRecord{JID: res.JID, Success: res.Success, Error: res.Error})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"linked": out})
}

// handleCommunityUnlinkGroup: POST /community/unlinkGroup/{instance} {communityJid, groupJid}.
func (s *Server) handleCommunityUnlinkGroup(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityUnlinkGroupReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if req.GroupJid == "" {
		s.writeError(w, http.StatusBadRequest, "groupJid is required")
		return
	}
	if err := s.backend.CommunityUnlinkGroup(r.Context(), inst, normalizeJID(jid), normalizeJID(req.GroupJid)); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityLinkedGroups: GET /community/linkedGroups/{instance}?communityJid=.
func (s *Server) handleCommunityLinkedGroups(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := r.URL.Query().Get("communityJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid query param is required")
		return
	}
	links, err := s.backend.CommunityLinkedGroups(r.Context(), inst, normalizeJID(jid))
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]communityLinkRecord, 0, len(links))
	for _, l := range links {
		out = append(out, communityLinkRecord{JID: l.JID, Subject: l.Subject})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleCommunityRequestList: GET /community/requestList/{instance}?communityJid=.
func (s *Server) handleCommunityRequestList(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := r.URL.Query().Get("communityJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid query param is required")
		return
	}
	reqs, err := s.backend.CommunityRequestList(r.Context(), inst, normalizeJID(jid))
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]communityRequestRecord, 0, len(reqs))
	for _, rq := range reqs {
		out = append(out, communityRequestRecord{JID: rq.JID, Attrs: rq.Attrs})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleCommunityRequestUpdate: POST /community/requestUpdate/{instance}
// {communityJid, participants[], action approve|reject}.
func (s *Server) handleCommunityRequestUpdate(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityRequestUpdateReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Action {
	case "approve", "reject":
	default:
		s.writeError(w, http.StatusBadRequest, "action must be approve|reject")
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if len(req.Participants) == 0 {
		s.writeError(w, http.StatusBadRequest, "participants is required")
		return
	}
	parts := make([]string, 0, len(req.Participants))
	for _, p := range req.Participants {
		if nj := normalizeJID(p); nj != "" {
			parts = append(parts, nj)
		}
	}
	if len(parts) == 0 {
		s.writeError(w, http.StatusBadRequest, "participants is required")
		return
	}
	res, err := s.backend.CommunityRequestUpdate(r.Context(), inst, normalizeJID(jid), parts, req.Action)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]participantResult, 0, len(res))
	for _, rr := range res {
		out = append(out, participantResult{JID: rr.JID, Status: rr.Status})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleCommunityUpdateParticipant: POST /community/updateParticipant/{instance}
// {communityJid, participants[], action add|remove|promote|demote}.
func (s *Server) handleCommunityUpdateParticipant(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityParticipantReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Action {
	case "add", "remove", "promote", "demote":
	default:
		s.writeError(w, http.StatusBadRequest, "action must be add|remove|promote|demote")
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if len(req.Participants) == 0 {
		s.writeError(w, http.StatusBadRequest, "participants is required")
		return
	}
	parts := make([]string, 0, len(req.Participants))
	for _, p := range req.Participants {
		if nj := normalizeJID(p); nj != "" {
			parts = append(parts, nj)
		}
	}
	if len(parts) == 0 {
		s.writeError(w, http.StatusBadRequest, "participants is required")
		return
	}
	res, err := s.backend.CommunityParticipantsUpdate(r.Context(), inst, normalizeJID(jid), parts, req.Action)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]participantResult, 0, len(res))
	for _, rr := range res {
		out = append(out, participantResult{JID: rr.JID, Status: rr.Status})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleCommunityJoinApprovalMode: POST /community/joinApprovalMode/{instance} {communityJid, mode on|off}.
func (s *Server) handleCommunityJoinApprovalMode(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityModeReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Mode {
	case "on", "off":
	default:
		s.writeError(w, http.StatusBadRequest, "mode must be on|off")
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if err := s.backend.CommunityJoinApprovalMode(r.Context(), inst, normalizeJID(jid), req.Mode); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityMemberAddMode: POST /community/memberAddMode/{instance} {communityJid, mode admin_add|all_member_add}.
func (s *Server) handleCommunityMemberAddMode(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityModeReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Mode {
	case "admin_add", "all_member_add":
	default:
		s.writeError(w, http.StatusBadRequest, "mode must be admin_add|all_member_add")
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if err := s.backend.CommunityMemberAddMode(r.Context(), inst, normalizeJID(jid), req.Mode); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityToggleEphemeral: POST /community/toggleEphemeral/{instance} {communityJid, expiration}.
func (s *Server) handleCommunityToggleEphemeral(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communityToggleEphemeralReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	switch req.Expiration {
	case 0, 86400, 604800, 7776000:
	default:
		s.writeError(w, http.StatusBadRequest, "expiration must be 0, 86400, 604800 or 7776000")
		return
	}
	if err := s.backend.CommunityToggleEphemeral(r.Context(), inst, normalizeJID(jid), req.Expiration); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunitySettingUpdate: POST /community/settingUpdate/{instance} {communityJid, action}.
// action is announcement|not_announcement|locked|unlocked and is passed to the
// library verbatim — these are the literal wire tags the server expects.
func (s *Server) handleCommunitySettingUpdate(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req communitySettingReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	switch req.Action {
	case "announcement", "not_announcement", "locked", "unlocked":
	default:
		s.writeError(w, http.StatusBadRequest, "action must be announcement|not_announcement|locked|unlocked")
		return
	}
	jid := communityJidFrom(r, req.CommunityJid)
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid is required")
		return
	}
	if err := s.backend.CommunitySettingUpdate(r.Context(), inst, normalizeJID(jid), req.Action); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleCommunityFetchAllParticipating: GET /community/fetchAllParticipating/{instance}.
// Lists the communities the account participates in; takes no parameters and
// returns the same camelCase [{jid,subject}] shape as linkedGroups.
func (s *Server) handleCommunityFetchAllParticipating(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	links, err := s.backend.CommunityFetchAllParticipating(r.Context(), inst)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	out := make([]communityLinkRecord, 0, len(links))
	for _, l := range links {
		out = append(out, communityLinkRecord{JID: l.JID, Subject: l.Subject})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleCommunityLeave: DELETE /community/leave/{instance}?communityJid=.
func (s *Server) handleCommunityLeave(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	jid := r.URL.Query().Get("communityJid")
	if jid == "" {
		s.writeError(w, http.StatusBadRequest, "communityJid query param is required")
		return
	}
	if err := s.backend.CommunityLeave(r.Context(), inst, normalizeJID(jid)); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// --- ManagerBackend implementations ---

func (b *ManagerBackend) CommunityCreate(ctx context.Context, name, subject, description string) (*wa.GroupInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityCreate(ctx, subject, description)
}

func (b *ManagerBackend) CommunityMetadata(ctx context.Context, name, communityJID string) (*wa.GroupInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityMetadata(ctx, communityJID)
}

func (b *ManagerBackend) CommunityUpdateSubject(ctx context.Context, name, communityJID, subject string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityUpdateSubject(ctx, communityJID, subject)
}

func (b *ManagerBackend) CommunityUpdateDescription(ctx context.Context, name, communityJID, description string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityUpdateDescription(ctx, communityJID, description, "")
}

func (b *ManagerBackend) CommunityLinkGroups(ctx context.Context, name, communityJID string, groupJIDs []string) ([]CommunityLinkResult, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	out := make([]CommunityLinkResult, 0, len(groupJIDs))
	for _, g := range groupJIDs {
		res := CommunityLinkResult{JID: g, Success: true}
		if lerr := c.LinkGroup(ctx, communityJID, g); lerr != nil {
			res.Success = false
			res.Error = lerr.Error()
		}
		out = append(out, res)
	}
	return out, nil
}

func (b *ManagerBackend) CommunityUnlinkGroup(ctx context.Context, name, communityJID, groupJID string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.UnlinkGroup(ctx, communityJID, groupJID)
}

func (b *ManagerBackend) CommunityLinkedGroups(ctx context.Context, name, communityJID string) ([]wa.GroupLinkInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityFetchLinkedGroups(ctx, communityJID)
}

func (b *ManagerBackend) CommunityRequestList(ctx context.Context, name, communityJID string) ([]wa.CommunityMembershipRequest, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityRequestParticipantsList(ctx, communityJID)
}

func (b *ManagerBackend) CommunityRequestUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	res, err := c.CommunityRequestParticipantsUpdate(ctx, communityJID, participants, action)
	if err != nil {
		return nil, err
	}
	out := make([]ParticipantResult, 0, len(res))
	for _, r := range res {
		out = append(out, ParticipantResult{JID: r.JID, Status: r.Status})
	}
	return out, nil
}

func (b *ManagerBackend) CommunityParticipantsUpdate(ctx context.Context, name, communityJID string, participants []string, action string) ([]ParticipantResult, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	res, err := c.CommunityParticipantsUpdate(ctx, communityJID, participants, action)
	if err != nil {
		return nil, err
	}
	out := make([]ParticipantResult, 0, len(res))
	for _, r := range res {
		out = append(out, ParticipantResult{JID: r.JID, Status: r.Status})
	}
	return out, nil
}

func (b *ManagerBackend) CommunityJoinApprovalMode(ctx context.Context, name, communityJID, mode string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityJoinApprovalMode(ctx, communityJID, mode)
}

func (b *ManagerBackend) CommunityMemberAddMode(ctx context.Context, name, communityJID, mode string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityMemberAddMode(ctx, communityJID, mode)
}

func (b *ManagerBackend) CommunityToggleEphemeral(ctx context.Context, name, communityJID string, expiration int) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityToggleEphemeral(ctx, communityJID, expiration)
}

func (b *ManagerBackend) CommunitySettingUpdate(ctx context.Context, name, communityJID, setting string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunitySettingUpdate(ctx, communityJID, setting)
}

func (b *ManagerBackend) CommunityLeave(ctx context.Context, name, communityJID string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.CommunityLeave(ctx, communityJID)
}

func (b *ManagerBackend) CommunityFetchAllParticipating(ctx context.Context, name string) ([]wa.GroupLinkInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityFetchAllParticipating(ctx)
}

func (b *ManagerBackend) CommunityCreateGroup(ctx context.Context, name, communityJID, subject string, participants []string) (*wa.GroupInfo, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityCreateGroup(ctx, communityJID, subject, participants)
}

func (b *ManagerBackend) CommunityLinkedGroupsParticipants(ctx context.Context, name, communityJID string) ([]string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return nil, err
	}
	return c.CommunityLinkedGroupsParticipants(ctx, communityJID)
}
