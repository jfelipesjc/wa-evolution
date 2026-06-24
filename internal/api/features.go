package api

import (
	"context"
	"net/http"
)

// --- ManagerBackend implementations (profile / status / newsletter) ---
// Each fetches the live *wa.Client and delegates to the lib method. The lib
// features were validated live; these routes surface them to HTTP callers
// (Chatwoot/workers).

func (b *ManagerBackend) ProfileSetName(ctx context.Context, name, displayName string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.UpdateProfileName(ctx, displayName)
}

func (b *ManagerBackend) ProfileSetStatus(ctx context.Context, name, status string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.UpdateProfileStatus(ctx, status)
}

func (b *ManagerBackend) PostStatus(ctx context.Context, name, text string, recipients []string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	return c.SendStatusText(ctx, text, recipients)
}

func (b *ManagerBackend) NewsletterCreate(ctx context.Context, name, channelName, description string) (string, error) {
	c, err := b.liveClient(name)
	if err != nil {
		return "", err
	}
	info, err := c.NewsletterCreate(ctx, channelName, description)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}
	return info.JID, nil
}

func (b *ManagerBackend) NewsletterFollow(ctx context.Context, name, jid string) error {
	c, err := b.liveClient(name)
	if err != nil {
		return err
	}
	return c.NewsletterFollow(ctx, jid)
}

// --- HTTP handlers ---

// handleProfileSetName: POST /chat/updateProfileName/{instance} {"name":"..."}.
func (s *Server) handleProfileSetName(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req struct {
		Name string `json:"name"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.backend.ProfileSetName(r.Context(), inst, req.Name); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "SUCCESS"})
}

// handleProfileSetStatus: POST /chat/updateProfileStatus/{instance} {"status":"..."}.
func (s *Server) handleProfileSetStatus(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req struct {
		Status string `json:"status"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := s.backend.ProfileSetStatus(r.Context(), inst, req.Status); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "SUCCESS"})
}

// handleSendStatus: POST /message/sendStatus/{instance}
// {"text":"...","recipients":["5512...@s.whatsapp.net", ...]}.
func (s *Server) handleSendStatus(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req struct {
		Text       string   `json:"text"`
		Recipients []string `json:"recipients"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Text == "" {
		s.writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if len(req.Recipients) == 0 {
		s.writeError(w, http.StatusBadRequest, "recipients is required")
		return
	}
	rcpts := make([]string, len(req.Recipients))
	for i, r := range req.Recipients {
		rcpts[i] = normalizeJID(r)
	}
	id, err := s.backend.PostStatus(r.Context(), inst, req.Text, rcpts)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "PENDING"})
}

// handleNewsletterCreate: POST /newsletter/create/{instance}
// {"name":"...","description":"..."}.
func (s *Server) handleNewsletterCreate(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	jid, err := s.backend.NewsletterCreate(r.Context(), inst, req.Name, req.Description)
	if err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"jid": jid})
}

// handleNewsletterFollow: POST /newsletter/follow/{instance} {"jid":"...@newsletter"}.
func (s *Server) handleNewsletterFollow(w http.ResponseWriter, r *http.Request) {
	inst := r.PathValue("instance")
	var req struct {
		JID string `json:"jid"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.JID == "" {
		s.writeError(w, http.StatusBadRequest, "jid is required")
		return
	}
	if err := s.backend.NewsletterFollow(r.Context(), inst, req.JID); err != nil {
		s.writeSendError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "SUCCESS"})
}
