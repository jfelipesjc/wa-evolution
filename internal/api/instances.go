package api

import (
	"errors"
	"net/http"
	"sort"
)

// handleCreateInstance: POST /instance/create {instanceName, webhookUrl?}.
func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req createInstanceReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.InstanceName == "" {
		s.writeError(w, http.StatusBadRequest, "instanceName is required")
		return
	}
	if err := s.backend.Create(req.InstanceName); err != nil {
		s.writeError(w, http.StatusConflict, err.Error())
		return
	}
	if req.WebhookURL != "" {
		s.dispatcher.set(req.InstanceName, req.WebhookURL)
	}
	status := s.backend.Status()[req.InstanceName]
	if status == "" {
		status = "close"
	}
	s.writeJSON(w, http.StatusCreated, createInstanceResp{
		Instance: instanceInfo{InstanceName: req.InstanceName, ConnectionStatus: status},
	})
}

// handleConnect: GET /instance/connect/{instance} -> {code, base64}.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	code, err := s.backend.Connect(r.Context(), name)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			s.writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b64, dataURI := qrPNGBase64(code)
	// Evolution clients use `code` as the scannable string and `base64` as the
	// renderable data URI. We expose the raw base64 PNG via `code` when a QR is
	// present (so a worker can decode it), and the data URI via `base64`.
	respCode := code
	if b64 != "" {
		respCode = b64
	}
	s.writeJSON(w, http.StatusOK, connectResp{
		Code:       respCode,
		Base64:     dataURI,
		Instance:   name,
		ConnStatus: s.backend.Status()[name],
	})
}

// handleFetchInstances: GET /instance/fetchInstances -> [{instanceName, connectionStatus}].
func (s *Server) handleFetchInstances(w http.ResponseWriter, r *http.Request) {
	status := s.backend.Status()
	names := make([]string, 0, len(status))
	for name := range status {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]instanceInfo, 0, len(names))
	for _, name := range names {
		out = append(out, instanceInfo{InstanceName: name, ConnectionStatus: status[name]})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleDelete: DELETE /instance/delete/{instance}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if err := s.backend.Delete(name); err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			s.writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dispatcher.remove(name)
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleLogout: GET /instance/logout/{instance}.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if err := s.backend.Logout(name); err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			s.writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, statusResp{Status: "SUCCESS"})
}

// handleSetWebhook: POST /webhook/set/{instance} {url} or {webhook:{url}}.
func (s *Server) handleSetWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var req setWebhookReq
	if !s.decodeJSON(w, r, &req) {
		return
	}
	url := req.URL
	if url == "" && req.Webhook != nil {
		url = req.Webhook.URL
	}
	if url == "" {
		s.writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	s.dispatcher.set(name, url)
	var resp setWebhookResp
	resp.Webhook.URL = url
	s.writeJSON(w, http.StatusOK, resp)
}
