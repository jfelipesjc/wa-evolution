package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSendPoll(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendPoll/bot1", testKey, sendPollReq{
		Number: "5512999", Name: "Lunch?", Values: []string{"Pizza", "Sushi"}, SelectableCount: 1,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var sr sendResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if sr.Key.ID != "MSGID-POLL" {
		t.Fatalf("key.id = %q", sr.Key.ID)
	}
	if len(fb.polls) != 1 || fb.polls[0].pollName != "Lunch?" || len(fb.polls[0].options) != 2 {
		t.Fatalf("poll recorded %+v", fb.polls)
	}
}

func TestSendPoll_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendPoll/bot1", testKey, sendPollReq{Number: "5512", Name: "x", Values: []string{"only"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSendSticker(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	b64 := base64.StdEncoding.EncodeToString([]byte("webp-bytes"))
	rec := do(t, h, "POST", "/message/sendSticker/bot1", testKey, sendStickerReq{Number: "5512999", Sticker: b64})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSendWhatsAppAudio(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	b64 := base64.StdEncoding.EncodeToString([]byte("ogg-bytes"))
	rec := do(t, h, "POST", "/message/sendWhatsAppAudio/bot1", testKey, sendAudioReq{Number: "5512999", Audio: b64})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateWithNumber_PairingCode(t *testing.T) {
	fb := newFakeBackend()
	h := newTestServer(t, fb)
	// create with a number -> CreateWithNumber receives the sanitized-ish number.
	rec := do(t, h, "POST", "/instance/create", testKey, createInstanceReq{InstanceName: "bot1", Number: "+55 12 99999-8888"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if fb.lastCreateNumber != "+55 12 99999-8888" {
		t.Fatalf("CreateWithNumber number = %q", fb.lastCreateNumber)
	}
	// connect WITHOUT ?number -> QR only, never a (stale) pairing code.
	fb.qr = "2@abc,def"
	fb.pairingCode = "STALE-9999"
	rec = do(t, h, "GET", "/instance/connect/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var cr connectResp
	_ = json.Unmarshal(rec.Body.Bytes(), &cr)
	if cr.PairingCode != "" {
		t.Fatalf("QR path leaked pairingCode = %q, want empty", cr.PairingCode)
	}
	if cr.Code == "" {
		t.Fatalf("QR path returned no code")
	}
	// connect WITH ?number -> pairing code via RequestPairingCode.
	fb.pairingCode = "ABCD-1234"
	rec = do(t, h, "GET", "/instance/connect/bot1?number=5512999998888", testKey, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &cr)
	if cr.PairingCode != "ABCD-1234" {
		t.Fatalf("?number pairingCode = %q, want ABCD-1234", cr.PairingCode)
	}
	if fb.lastReqPairNumber != "5512999998888" {
		t.Fatalf("RequestPairingCode number = %q", fb.lastReqPairNumber)
	}
}

func TestCreate_NoNumber_UsesQR(t *testing.T) {
	fb := newFakeBackend()
	fb.qr = "2@abc,def"
	h := newTestServer(t, fb)
	_ = do(t, h, "POST", "/instance/create", testKey, createInstanceReq{InstanceName: "bot1"})
	if fb.lastCreateNumber != "" {
		t.Fatalf("expected no number, got %q", fb.lastCreateNumber)
	}
	rec := do(t, h, "GET", "/instance/connect/bot1", testKey, nil)
	var cr connectResp
	_ = json.Unmarshal(rec.Body.Bytes(), &cr)
	if cr.PairingCode != "" || cr.Code == "" {
		t.Fatalf("expected QR (code set, no pairingCode), got %+v", cr)
	}
}

func TestSendPtv(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	b64 := base64.StdEncoding.EncodeToString([]byte("mp4-bytes"))
	rec := do(t, h, "POST", "/message/sendPtv/bot1", testKey, sendPtvReq{Number: "5512999", Video: b64})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var sr sendResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if sr.Key.ID != "MSGID-PTV" {
		t.Fatalf("key.id = %q", sr.Key.ID)
	}
}

func TestSendPtv_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/sendPtv/bot1", testKey, sendPtvReq{Number: "5512"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestArchiveChat(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/archiveChat/bot1", testKey, archiveChatReq{Chat: "5512999", Archive: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.archives) != 1 || fb.archives[0].jid != "5512999@s.whatsapp.net" || !fb.archives[0].archive {
		t.Fatalf("archive recorded %+v", fb.archives)
	}
}

func TestUpdateBlockStatus(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/updateBlockStatus/bot1", testKey, updateBlockStatusReq{Number: "5512999", Status: "block"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.blocks) != 1 || !fb.blocks[0].block || fb.blocks[0].jid != "5512999@s.whatsapp.net" {
		t.Fatalf("block recorded %+v", fb.blocks)
	}
	rec = do(t, h, "POST", "/chat/updateBlockStatus/bot1", testKey, updateBlockStatusReq{Number: "5512", Status: "explode"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status = %d, want 400", rec.Code)
	}
}

func TestFetchProfilePicture(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/fetchProfilePictureUrl/bot1", testKey, jidQueryReq{Number: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var pr profilePictureResp
	_ = json.Unmarshal(rec.Body.Bytes(), &pr)
	if pr.ProfilePic == "" || pr.WUID != "5512999@s.whatsapp.net" {
		t.Fatalf("resp = %+v", pr)
	}
}

func TestUpdatePrivacy(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/updatePrivacySettings/bot1", testKey, updatePrivacyReq{Name: "lastSeen", Value: "nobody"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.privacy) != 1 || fb.privacy[0][0] != "lastSeen" || fb.privacy[0][1] != "nobody" {
		t.Fatalf("privacy recorded %+v", fb.privacy)
	}
}

func TestFindChats(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.chats = []ChatInfoArg{{JID: "5512@s.whatsapp.net", Name: "Ana", Pinned: true}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/chat/findChats/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []chatRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Name != "Ana" || !out[0].Pinned {
		t.Fatalf("chats = %+v", out)
	}
}

func TestFindChatByRemoteJid(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.chats = []ChatInfoArg{{JID: "5512@s.whatsapp.net", Name: "Ana"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/findChatByRemoteJid/bot1", testKey, jidQueryReq{Number: "5512"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/chat/findChatByRemoteJid/bot1", testKey, jidQueryReq{Number: "9999"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing chat status = %d, want 404", rec.Code)
	}
}

func TestFindContacts(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.contactsList = []ContactArg{{JID: "5512@s.whatsapp.net", PushName: "Ana"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/chat/findContacts/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []contactRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].PushName != "Ana" {
		t.Fatalf("contacts = %+v", out)
	}
}

func TestGroupAcceptInvite(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/group/acceptInviteCode/bot1", testKey, acceptInviteReq{InviteCode: "ABC"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var ar acceptInviteResp
	_ = json.Unmarshal(rec.Body.Bytes(), &ar)
	if ar.GroupJID != "123@g.us" {
		t.Fatalf("groupJid = %q", ar.GroupJID)
	}
}

func TestGroupRevokeInvite(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "PUT", "/group/revokeInviteCode/bot1?groupJid=123@g.us", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var rr revokeInviteResp
	_ = json.Unmarshal(rec.Body.Bytes(), &rr)
	if rr.InviteCode != "NEWCODE" || rr.InviteURL != "https://chat.whatsapp.com/NEWCODE" {
		t.Fatalf("revoke = %+v", rr)
	}
}

func TestGroupInviteInfo(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/group/inviteInfo/bot1?inviteCode=ABC", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var g groupRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &g)
	if g.Subject != "Invited Group" {
		t.Fatalf("group = %+v", g)
	}
}

func TestGroupUpdateSubject(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "PUT", "/group/updateGroupSubject/bot1", testKey, updateGroupSubjectReq{GroupJID: "123@g.us", Subject: "New Name"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.groupSubjects) != 1 || fb.groupSubjects[0][1] != "New Name" {
		t.Fatalf("subjects = %+v", fb.groupSubjects)
	}
}

func TestGroupSendInvite(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/group/sendInvite/bot1", testKey, sendInviteReq{
		GroupJID: "123@g.us", Numbers: []string{"5512999"}, Description: "join us",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.groupInvites) != 1 || fb.groupInvites[0].numbers[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("invites = %+v", fb.groupInvites)
	}
}

func TestGroupToggleEphemeral(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "PUT", "/group/toggleEphemeral/bot1", testKey, toggleEphemeralReq{GroupJID: "123@g.us", Expiration: 604800})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupUpdateSetting(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	// Evolution action names are passed verbatim as the literal wire tags
	// (announcement/not_announcement/locked/unlocked); the lib accepts them.
	for _, tc := range []struct{ action, want string }{
		{"announcement", "announcement"},
		{"not_announcement", "not_announcement"},
		{"locked", "locked"},
		{"unlocked", "unlocked"},
	} {
		rec := do(t, h, "PUT", "/group/updateSetting/bot1", testKey, updateGroupSettingReq{GroupJID: "123@g.us", Action: tc.action})
		if rec.Code != http.StatusOK {
			t.Fatalf("action %q: status = %d; body=%s", tc.action, rec.Code, rec.Body.String())
		}
		if fb.lastGroupSetting != tc.want {
			t.Fatalf("action %q passed %q to lib; want %q", tc.action, fb.lastGroupSetting, tc.want)
		}
	}
	rec := do(t, h, "PUT", "/group/updateSetting/bot1", testKey, updateGroupSettingReq{GroupJID: "123@g.us", Action: "nope"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action status = %d, want 400", rec.Code)
	}
}

func TestFindLabels(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.labels = []LabelArg{{ID: "1", Name: "Cliente", Color: "5"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/label/findLabels/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []labelRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Name != "Cliente" {
		t.Fatalf("labels = %+v", out)
	}
}

func TestHandleLabel(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/label/handleLabel/bot1", testKey, handleLabelReq{Number: "5512999", LabelID: "1", Action: "add"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.labelOps) != 1 || fb.labelOps[0].action != "add" || fb.labelOps[0].chatJID != "5512999@s.whatsapp.net" {
		t.Fatalf("labelOps = %+v", fb.labelOps)
	}
}

func TestOfferCall(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/call/offer/bot1", testKey, offerCallReq{Number: "5512999", IsVideo: true})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var or offerCallResp
	_ = json.Unmarshal(rec.Body.Bytes(), &or)
	if or.CallID != "CALLID-1" {
		t.Fatalf("callId = %q", or.CallID)
	}
}

func TestGetCatalog(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.products = []ProductArg{{ID: "p1", Name: "Plano", Price: 2990, Currency: "BRL"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/business/getCatalog/bot1", testKey, getCatalogReq{Number: "5512999", Limit: 10})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []productRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Name != "Plano" || out[0].Price != 2990 {
		t.Fatalf("products = %+v", out)
	}
}

func TestSetPresenceGlobal(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/instance/setPresence/bot1", testKey, setPresenceReq{Presence: "available"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.presences) != 1 || fb.presences[0].presence != "available" {
		t.Fatalf("presence = %+v", fb.presences)
	}
}

func TestRestart(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "PUT", "/instance/restart/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "PUT", "/instance/restart/ghost", testKey, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ghost restart status = %d, want 404", rec.Code)
	}
}

func TestSettingsSetAndFind(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/settings/set/bot1", testKey, settingsBody{RejectCall: true, MsgCall: "estou ocupado"})
	if rec.Code != http.StatusOK {
		t.Fatalf("set status = %d; body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/settings/find/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("find status = %d", rec.Code)
	}
	var got settingsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.RejectCall || got.MsgCall != "estou ocupado" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestProxySetAndFind(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/proxy/set/bot1", testKey, proxyBody{Enabled: true, Host: "1.2.3.4", Port: "8080", Protocol: "http"})
	if rec.Code != http.StatusOK {
		t.Fatalf("set status = %d; body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/proxy/find/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("find status = %d", rec.Code)
	}
	var got proxyBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Enabled || got.Host != "1.2.3.4" {
		t.Fatalf("proxy = %+v", got)
	}
}

func TestGetBase64FromMedia(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.mediaBytes = []byte("decrypted-jpeg")
	fb.mediaMime = "image/jpeg"
	h := newTestServer(t, fb)
	var body getBase64Req
	body.Message = &struct {
		Key messageKey `json:"key"`
	}{Key: messageKey{RemoteJID: "5512999", ID: "WAMID1"}}
	rec := do(t, h, "POST", "/chat/getBase64FromMediaMessage/bot1", testKey, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var gr getBase64Resp
	_ = json.Unmarshal(rec.Body.Bytes(), &gr)
	if gr.Mimetype != "image/jpeg" {
		t.Fatalf("mimetype = %q", gr.Mimetype)
	}
	got, _ := base64.StdEncoding.DecodeString(gr.Base64)
	if string(got) != "decrypted-jpeg" {
		t.Fatalf("base64 decodes to %q", got)
	}
}

func TestGetBase64FromMedia_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/getBase64FromMediaMessage/bot1", testKey, getBase64Req{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMarkChatUnread(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/markChatUnread/bot1", testKey, markChatUnreadReq{Chat: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.unreads) != 1 || fb.unreads[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("unreads = %+v", fb.unreads)
	}
}

func TestPinChat(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/pinChat/bot1", testKey, pinChatReq{Chat: "5512999", Pin: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.pins) != 1 || fb.pins[0].jid != "5512999@s.whatsapp.net" || !fb.pins[0].pin {
		t.Fatalf("pins recorded %+v", fb.pins)
	}
}

func TestPinChat_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/pinChat/bot1", testKey, pinChatReq{Pin: true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMuteChat(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/muteChat/bot1", testKey, muteChatReq{Chat: "5512999", Duration: 3600})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.mutes) != 1 || fb.mutes[0].jid != "5512999@s.whatsapp.net" || fb.mutes[0].seconds != 3600 {
		t.Fatalf("mutes recorded %+v", fb.mutes)
	}
}

func TestStarMessage(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/starMessage/bot1", testKey, starMessageReq{Number: "5512999", MessageID: "MSG1", FromMe: true, Star: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.stars) != 1 || fb.stars[0].jid != "5512999@s.whatsapp.net" || fb.stars[0].msgID != "MSG1" || !fb.stars[0].fromMe || !fb.stars[0].star {
		t.Fatalf("stars recorded %+v", fb.stars)
	}
}

func TestStarMessage_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/message/starMessage/bot1", testKey, starMessageReq{Number: "5512999"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestClearChat(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/clearChat/bot1", testKey, clearChatReq{Chat: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.clears) != 1 || fb.clears[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("clears = %+v", fb.clears)
	}
}

func TestDeleteChat(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/deleteChat/bot1", testKey, deleteChatReq{Chat: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.chatDeletes) != 1 || fb.chatDeletes[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("chatDeletes = %+v", fb.chatDeletes)
	}
}

func TestResyncAppState(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/resyncAppState/bot1", testKey, resyncAppStateReq{Collections: []string{"regular", "critical_block"}, Fresh: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.resyncs) != 1 || len(fb.resyncs[0].collections) != 2 || !fb.resyncs[0].fresh {
		t.Fatalf("resyncs = %+v", fb.resyncs)
	}
}

func TestResyncAppState_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/resyncAppState/bot1", testKey, resyncAppStateReq{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestFindStatusMessage(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.messages["status@broadcast"] = []StoredMsg{{ID: "S1", ChatJID: "status@broadcast", Text: "story", Type: "image"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/findStatusMessage/bot1", testKey, findMessagesReq{Limit: 5})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var fr findMessagesResp
	_ = json.Unmarshal(rec.Body.Bytes(), &fr)
	if len(fr.Messages.Records) != 1 || fr.Messages.Records[0].Key.ID != "S1" {
		t.Fatalf("records = %+v", fr.Messages.Records)
	}
}

func TestGetCollections(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.products = []ProductArg{{ID: "p1", Name: "Plano", Price: 2990, Currency: "BRL"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/business/getCollections/bot1", testKey, getCatalogReq{Number: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []collectionRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || len(out[0].Products) != 1 || out[0].Products[0].Name != "Plano" {
		t.Fatalf("collections = %+v", out)
	}
}

func TestDeleteMessageForEveryone_ChatAlias(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/chat/deleteMessageForEveryone/bot1", testKey, deleteMessageReq{
		Key: &messageKey{RemoteJID: "5512999", ID: "DEL1", FromMe: true},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.deletes) != 1 {
		t.Fatalf("delete not recorded via chat alias")
	}
}
