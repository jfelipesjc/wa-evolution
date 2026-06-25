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
	var cerr error
	if req.Number != "" {
		cerr = s.backend.CreateWithNumber(req.InstanceName, req.Number)
	} else {
		cerr = s.backend.Create(req.InstanceName)
	}
	if cerr != nil {
		s.writeError(w, http.StatusConflict, cerr.Error())
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
	// ?number=<phone> -> request an 8-char pairing CODE for that number (Evolution
	// behaviour). The number is supplied/confirmed by the user at connect time.
	if num := r.URL.Query().Get("number"); num != "" {
		pc, err := s.backend.RequestPairingCode(r.Context(), name, num)
		if err != nil {
			s.writeSendError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, connectResp{PairingCode: pc, Instance: name, ConnStatus: s.backend.Status()[name]})
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
	// The QR path (no ?number=) always returns a QR. Pairing codes are returned
	// only by the explicit ?number=<phone> path (handled above), so a stale cached
	// code never leaks into a QR request.
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
		number, pushName := s.backend.OwnProfile(name)
		info := instanceInfo{InstanceName: name, ConnectionStatus: status[name], ProfileName: pushName, Number: s.backend.PairingNumber(name)}
		if number != "" {
			info.OwnerJid = number + "@s.whatsapp.net"
		}
		out = append(out, info)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleConnectionState: GET /instance/connectionState/{instance}. Reports a
// single instance's connection state in Evolution's shape:
// {"instance":{"instanceName":"...","state":"open|close|connecting"}}. Workers
// poll this to detect dropped sessions and trigger a reconnect.
func (s *Server) handleConnectionState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	state := s.backend.Status()[name]
	if state == "" {
		state = "close"
	}
	s.writeJSON(w, http.StatusOK, connectionStateResp{
		Instance: connectionStateInfo{InstanceName: name, State: state},
	})
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

// handleFindWebhook: GET /webhook/find/{instance} -> {enabled, url}. Reports the
// persisted/configured webhook URL for the instance.
func (s *Server) handleFindWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("instance")
	if !s.backend.Exists(name) {
		s.writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	url := s.dispatcher.url(name)
	s.writeJSON(w, http.StatusOK, findWebhookResp{Enabled: url != "", URL: url})
}
