// Package api is an HTTP/JSON service that wraps the multi-session WhatsApp stack
// (internal/manager + internal/client + internal/store) behind a contract that
// mirrors the Evolution API v2 — so the user's existing Chatwoot / worker
// integrations can talk to it unchanged.
//
// It is built on the standard library only (net/http + http.ServeMux); no router
// dependency. All routes require the global apikey (header "apikey"). Inbound
// WhatsApp events are pushed to a per-instance webhook URL in Evolution shape
// ({event, instance, data}); see webhook.go.
//
// The handlers depend on the Backend interface (backend.go), which the real
// Manager satisfies via managerBackend and which a fake satisfies in tests, so
// the entire HTTP surface is exercised offline (httptest), reusing the
// session-injection testing pattern of internal/manager. WebSocket streaming is
// intentionally not implemented (net/http has no WS server); the webhook push is
// the supported ingestion path. See the design spec.
package api

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"rsc.io/qr"
)

// Server is the HTTP service. Construct it with New and serve s.Handler().
type Server struct {
	apikey  string
	backend Backend
	mux     *http.ServeMux

	// webhooks maps instance name -> configured webhook URL (set on create or via
	// /webhook/set). Guarded by the dispatcher's mutex (see webhook.go).
	dispatcher *webhookDispatcher

	// cfg holds per-instance Evolution settings/proxy blobs. These are app-level
	// config Evolution stores and echoes back; this build keeps them in memory and
	// returns them on find (it does not yet act on rejectCall/alwaysOnline/proxy).
	cfg *configStore

	logger *log.Logger
}

// Options configures a Server.
type Options struct {
	APIKey  string
	Backend Backend
	// HTTPClient overrides the webhook delivery client (tests inject one).
	HTTPClient httpDoer
	// Logger overrides the default (log.Default()).
	Logger *log.Logger
	// WebhookDir, when set, is the directory where per-instance webhook URLs are
	// persisted (as <instance>.webhook sidecar files) so configured webhooks
	// survive a restart. Empty keeps webhooks in memory only.
	WebhookDir string
}

// New constructs a Server. apikey and backend are required.
func New(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	s := &Server{
		apikey:     opts.APIKey,
		backend:    opts.Backend,
		mux:        http.NewServeMux(),
		dispatcher: newWebhookDispatcher(opts.HTTPClient, logger),
		cfg:        newConfigStore(),
		logger:     logger,
	}
	s.dispatcher.setDir(opts.WebhookDir)
	s.routes()
	return s
}

// Dispatcher exposes the webhook dispatcher so the host can pump manager events
// into it (see RunEventPump / cmd/wa-server).
func (s *Server) Dispatcher() *webhookDispatcher { return s.dispatcher }

// Handler returns the http.Handler (auth middleware wrapping the mux).
func (s *Server) Handler() http.Handler { return s.authMiddleware(s.mux) }

// routes wires every endpoint. Go 1.22 ServeMux supports method+wildcard patterns.
func (s *Server) routes() {
	s.mux.HandleFunc("POST /instance/create", s.handleCreateInstance)
	s.mux.HandleFunc("GET /instance/connect/{instance}", s.handleConnect)
	s.mux.HandleFunc("GET /instance/fetchInstances", s.handleFetchInstances)
	s.mux.HandleFunc("DELETE /instance/delete/{instance}", s.handleDelete)
	s.mux.HandleFunc("GET /instance/logout/{instance}", s.handleLogout)
	s.mux.HandleFunc("GET /instance/connectionState/{instance}", s.handleConnectionState)

	s.mux.HandleFunc("POST /webhook/set/{instance}", s.handleSetWebhook)
	s.mux.HandleFunc("GET /webhook/find/{instance}", s.handleFindWebhook)

	s.mux.HandleFunc("POST /message/sendText/{instance}", s.handleSendText)
	s.mux.HandleFunc("POST /message/sendMedia/{instance}", s.handleSendMedia)
	s.mux.HandleFunc("POST /message/sendReaction/{instance}", s.handleSendReaction)
	s.mux.HandleFunc("POST /message/markMessageAsRead/{instance}", s.handleMarkRead)
	s.mux.HandleFunc("POST /message/deleteMessage/{instance}", s.handleDeleteMessage)
	s.mux.HandleFunc("POST /message/editMessage/{instance}", s.handleEditMessage)
	s.mux.HandleFunc("POST /message/sendButtons/{instance}", s.handleSendButtons)
	s.mux.HandleFunc("POST /message/sendList/{instance}", s.handleSendList)
	s.mux.HandleFunc("POST /message/sendLocation/{instance}", s.handleSendLocation)
	s.mux.HandleFunc("POST /message/sendContact/{instance}", s.handleSendContact)

	s.mux.HandleFunc("POST /chat/findMessages/{instance}", s.handleFindMessages)
	s.mux.HandleFunc("POST /chat/whatsappNumbers/{instance}", s.handleWhatsAppNumbers)
	// /chat/check is an Evolution alias of /chat/whatsappNumbers.
	s.mux.HandleFunc("POST /chat/check/{instance}", s.handleWhatsAppNumbers)
	s.mux.HandleFunc("POST /chat/sendPresence/{instance}", s.handleSendPresence)

	s.mux.HandleFunc("GET /group/fetchAllGroups/{instance}", s.handleFetchAllGroups)
	s.mux.HandleFunc("GET /group/groupMetadata/{instance}", s.handleGroupMetadata)
	s.mux.HandleFunc("POST /group/create/{instance}", s.handleGroupCreate)
	s.mux.HandleFunc("POST /group/updateParticipant/{instance}", s.handleUpdateParticipant)
	s.mux.HandleFunc("GET /group/inviteCode/{instance}", s.handleInviteCode)
	s.mux.HandleFunc("POST /group/leave/{instance}", s.handleLeaveGroup)

	// profile / status / newsletter
	s.mux.HandleFunc("POST /chat/updateProfileName/{instance}", s.handleProfileSetName)
	s.mux.HandleFunc("POST /chat/updateProfileStatus/{instance}", s.handleProfileSetStatus)
	s.mux.HandleFunc("POST /message/sendStatus/{instance}", s.handleSendStatus)
	s.mux.HandleFunc("POST /newsletter/create/{instance}", s.handleNewsletterCreate)
	s.mux.HandleFunc("POST /newsletter/follow/{instance}", s.handleNewsletterFollow)

	// --- extended Evolution-parity surface ---

	// instance
	s.mux.HandleFunc("PUT /instance/restart/{instance}", s.handleRestart)
	s.mux.HandleFunc("POST /instance/setPresence/{instance}", s.handleSetPresence)

	// message
	s.mux.HandleFunc("POST /message/sendPoll/{instance}", s.handleSendPoll)
	s.mux.HandleFunc("POST /message/sendSticker/{instance}", s.handleSendSticker)
	s.mux.HandleFunc("POST /message/sendWhatsAppAudio/{instance}", s.handleSendWhatsAppAudio)
	s.mux.HandleFunc("POST /message/sendPtv/{instance}", s.handleSendPtv)

	// chat
	s.mux.HandleFunc("POST /chat/archiveChat/{instance}", s.handleArchiveChat)
	s.mux.HandleFunc("POST /chat/deleteMessageForEveryone/{instance}", s.handleDeleteMessage)
	s.mux.HandleFunc("POST /chat/updateMessage/{instance}", s.handleEditMessage)
	s.mux.HandleFunc("POST /chat/fetchProfilePictureUrl/{instance}", s.handleFetchProfilePicture)
	s.mux.HandleFunc("POST /chat/fetchBusinessProfile/{instance}", s.handleFetchBusinessProfile)
	s.mux.HandleFunc("POST /chat/fetchProfile/{instance}", s.handleFetchProfile)
	s.mux.HandleFunc("GET /chat/fetchPrivacySettings/{instance}", s.handleFetchPrivacy)
	s.mux.HandleFunc("POST /chat/updatePrivacySettings/{instance}", s.handleUpdatePrivacy)
	s.mux.HandleFunc("POST /chat/updateBlockStatus/{instance}", s.handleUpdateBlockStatus)
	s.mux.HandleFunc("PUT /chat/updateProfilePicture/{instance}", s.handleUpdateProfilePicture)
	s.mux.HandleFunc("DELETE /chat/removeProfilePicture/{instance}", s.handleRemoveProfilePicture)
	s.mux.HandleFunc("POST /chat/findChats/{instance}", s.handleFindChats)
	s.mux.HandleFunc("GET /chat/findChats/{instance}", s.handleFindChats)
	s.mux.HandleFunc("POST /chat/findChatByRemoteJid/{instance}", s.handleFindChatByRemoteJid)
	s.mux.HandleFunc("POST /chat/findContacts/{instance}", s.handleFindContacts)
	s.mux.HandleFunc("GET /chat/findContacts/{instance}", s.handleFindContacts)
	s.mux.HandleFunc("POST /chat/getBase64FromMediaMessage/{instance}", s.handleGetBase64FromMedia)
	s.mux.HandleFunc("POST /chat/markChatUnread/{instance}", s.handleMarkChatUnread)
	s.mux.HandleFunc("POST /chat/findStatusMessage/{instance}", s.handleFindStatusMessage)

	// group
	s.mux.HandleFunc("GET /group/findGroupInfos/{instance}", s.handleGroupMetadata)
	s.mux.HandleFunc("GET /group/participants/{instance}", s.handleGroupParticipants)
	s.mux.HandleFunc("GET /group/inviteInfo/{instance}", s.handleGroupInviteInfo)
	s.mux.HandleFunc("POST /group/acceptInviteCode/{instance}", s.handleGroupAcceptInvite)
	s.mux.HandleFunc("PUT /group/revokeInviteCode/{instance}", s.handleGroupRevokeInvite)
	s.mux.HandleFunc("POST /group/sendInvite/{instance}", s.handleGroupSendInvite)
	s.mux.HandleFunc("PUT /group/updateGroupSubject/{instance}", s.handleGroupUpdateSubject)
	s.mux.HandleFunc("PUT /group/updateGroupDescription/{instance}", s.handleGroupUpdateDescription)
	s.mux.HandleFunc("PUT /group/updateGroupPicture/{instance}", s.handleGroupUpdatePicture)
	s.mux.HandleFunc("PUT /group/toggleEphemeral/{instance}", s.handleGroupToggleEphemeral)
	s.mux.HandleFunc("PUT /group/updateSetting/{instance}", s.handleGroupUpdateSetting)

	// labels
	s.mux.HandleFunc("GET /label/findLabels/{instance}", s.handleFindLabels)
	s.mux.HandleFunc("POST /label/handleLabel/{instance}", s.handleHandleLabel)

	// call
	s.mux.HandleFunc("POST /call/offer/{instance}", s.handleOfferCall)

	// business
	s.mux.HandleFunc("POST /business/getCatalog/{instance}", s.handleGetCatalog)
	s.mux.HandleFunc("POST /business/getCollections/{instance}", s.handleGetCollections)

	// settings (instance config; stored + echoed)
	s.mux.HandleFunc("POST /settings/set/{instance}", s.handleSetSettings)
	s.mux.HandleFunc("GET /settings/find/{instance}", s.handleFindSettings)

	// proxy (instance config; stored + echoed)
	s.mux.HandleFunc("POST /proxy/set/{instance}", s.handleSetProxy)
	s.mux.HandleFunc("GET /proxy/find/{instance}", s.handleFindProxy)
}

// authMiddleware enforces the global apikey on every request. The apikey is read
// from the "apikey" header (Evolution's convention). An empty configured apikey
// disables auth (development only).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apikey != "" {
			got := r.Header.Get("apikey")
			if got == "" {
				// Also accept Authorization: Bearer/Apikey <key> as a fallback.
				got = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			}
			if got != s.apikey {
				s.writeJSON(w, http.StatusUnauthorized, map[string]any{
					"status":  http.StatusUnauthorized,
					"error":   "Unauthorized",
					"message": "invalid or missing apikey",
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- JSON helpers ---

func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Printf("api: encode response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	s.writeJSON(w, code, map[string]any{
		"status":  code,
		"error":   http.StatusText(code),
		"message": msg,
	})
}

// decodeJSON reads and unmarshals the request body into v. It returns false (and
// writes a 400) if the body is malformed.
func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		s.writeError(w, http.StatusBadRequest, "empty body")
		return false
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// tryDecodeJSON decodes an optional request body without writing an error
// response on failure (used by endpoints whose body is optional, e.g. paging
// params). It returns false if nothing could be decoded.
func (s *Server) tryDecodeJSON(r *http.Request, v any) bool {
	if r.Body == nil {
		return false
	}
	return json.NewDecoder(r.Body).Decode(v) == nil
}

// qrPNGBase64 renders a QR string to a base64-encoded PNG and a data URI. It
// returns ("","") if the code is empty or fails to encode.
func qrPNGBase64(code string) (b64, dataURI string) {
	if code == "" {
		return "", ""
	}
	c, err := qr.Encode(code, qr.M)
	if err != nil {
		return "", ""
	}
	b64 = base64.StdEncoding.EncodeToString(c.PNG())
	return b64, "data:image/png;base64," + b64
}
