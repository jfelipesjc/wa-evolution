package api

import (
	"net/http"
	"testing"
)

// TestEvolutionCanonicalAliases verifies the canonical Evolution API path/method
// aliases are registered (route to a handler, not 404), so a strict Evolution
// client (n8n node, Chatwoot Evolution channel, Postman) gets the same behavior
// as the official server. A non-404 status proves the alias is wired; the exact
// success/validation code is covered by each handler's own test.
func TestEvolutionCanonicalAliases(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)

	cases := []struct {
		method, path string
		body         any
	}{
		{"POST", "/chat/markMessageAsRead/bot1", map[string]any{"readMessages": []map[string]any{{"id": "X", "remoteJid": "5512999@s.whatsapp.net", "fromMe": false}}}},
		{"DELETE", "/group/leaveGroup/bot1?groupJid=123@g.us", nil},
		{"DELETE", "/instance/logout/bot1", nil},
		{"POST", "/instance/restart/bot1", nil},
		{"GET", "/group/acceptInviteCode/bot1?inviteCode=abc", nil},
		{"POST", "/group/updateGroupSubject/bot1?groupJid=123@g.us", map[string]string{"subject": "x"}},
		{"POST", "/group/updateSetting/bot1?groupJid=123@g.us", map[string]string{"action": "locked"}},
		{"POST", "/group/revokeInviteCode/bot1?groupJid=123@g.us", nil},
		{"DELETE", "/chat/deleteMessageForEveryone/bot1", map[string]any{"id": "X", "remoteJid": "5512999@s.whatsapp.net", "fromMe": true}},
		{"POST", "/chat/updateProfilePicture/bot1", map[string]string{"picture": "data"}},
	}
	for _, tc := range cases {
		rec := do(t, h, tc.method, tc.path, testKey, tc.body)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s -> 404 (canonical Evolution alias not routed)", tc.method, tc.path)
		}
	}
}
