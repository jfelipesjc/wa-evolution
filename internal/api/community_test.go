package api

import (
	"encoding/json"
	"net/http"
	"testing"

	wa "github.com/jfelipesjc/wa-go/wa"
)

func TestCommunityCreate(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/create/bot1", testKey, communityCreateReq{Subject: "Bairro", Description: "vizinhos"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var cr communityResp
	_ = json.Unmarshal(rec.Body.Bytes(), &cr)
	if cr.Subject != "Bairro" {
		t.Fatalf("resp = %+v", cr)
	}
	if len(fb.communityCreates) != 1 || fb.communityCreates[0].subject != "Bairro" || fb.communityCreates[0].description != "vizinhos" {
		t.Fatalf("create recorded %+v", fb.communityCreates)
	}
}

func TestCommunityCreate_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/create/bot1", testKey, communityCreateReq{Description: "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityFind(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.communityInfo = &wa.GroupInfo{
		JID: "120363@g.us", Subject: "Bairro", Owner: "5512@s.whatsapp.net", Desc: "desc", Creation: 100,
		Participants: []wa.GroupParticipant{{JID: "5512@s.whatsapp.net", IsAdmin: true, IsSuperAdmin: true}},
	}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/findCommunity/bot1?communityJid=120363@g.us", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var cr communityResp
	_ = json.Unmarshal(rec.Body.Bytes(), &cr)
	if cr.Subject != "Bairro" || cr.Description != "desc" || len(cr.Participants) != 1 {
		t.Fatalf("resp = %+v", cr)
	}
	if !cr.Participants[0].IsAdmin || !cr.Participants[0].IsSuperAdmin {
		t.Fatalf("participant flags = %+v", cr.Participants[0])
	}
}

func TestCommunityFind_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/findCommunity/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityUpdateSubject(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateSubject/bot1", testKey, communityUpdateSubjectReq{CommunityJid: "120363@g.us", Subject: "Novo Nome"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communitySubjects) != 1 || fb.communitySubjects[0][0] != "120363@g.us" || fb.communitySubjects[0][1] != "Novo Nome" {
		t.Fatalf("subjects = %+v", fb.communitySubjects)
	}
}

func TestCommunityUpdateSubject_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateSubject/bot1", testKey, communityUpdateSubjectReq{CommunityJid: "120363@g.us"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no subject)", rec.Code)
	}
	rec = do(t, h, "POST", "/community/updateSubject/bot1", testKey, communityUpdateSubjectReq{Subject: "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no communityJid)", rec.Code)
	}
}

func TestCommunityUpdateDescription(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateDescription/bot1", testKey, communityUpdateDescriptionReq{CommunityJid: "120363@g.us", Description: "regras"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityDescs) != 1 || fb.communityDescs[0][0] != "120363@g.us" || fb.communityDescs[0][1] != "regras" {
		t.Fatalf("descs = %+v", fb.communityDescs)
	}
}

func TestCommunityUpdateDescription_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateDescription/bot1", testKey, communityUpdateDescriptionReq{Description: "regras"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no communityJid)", rec.Code)
	}
}

func TestCommunityLinkGroup(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/linkGroup/bot1", testKey, communityLinkGroupReq{
		CommunityJid: "120363@g.us", Groups: []string{"111@g.us", "222@g.us"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Linked []communityLinkResultRecord `json:"linked"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Linked) != 2 || !out.Linked[0].Success || out.Linked[0].JID != "111@g.us" {
		t.Fatalf("linked = %+v", out.Linked)
	}
	if len(fb.communityLinks) != 2 || fb.communityLinks[0].op != "link" || fb.communityLinks[0].communityJID != "120363@g.us" {
		t.Fatalf("links = %+v", fb.communityLinks)
	}
}

func TestCommunityLinkGroup_SingleGroupJid(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/linkGroup/bot1", testKey, communityLinkGroupReq{CommunityJid: "120363@g.us", GroupJid: "333@g.us"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityLinks) != 1 || fb.communityLinks[0].groupJID != "333@g.us" {
		t.Fatalf("links = %+v", fb.communityLinks)
	}
}

func TestCommunityLinkGroup_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/linkGroup/bot1", testKey, communityLinkGroupReq{CommunityJid: "120363@g.us"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no groups)", rec.Code)
	}
}

func TestCommunityUnlinkGroup(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/unlinkGroup/bot1", testKey, communityUnlinkGroupReq{CommunityJid: "120363@g.us", GroupJid: "111@g.us"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityLinks) != 1 || fb.communityLinks[0].op != "unlink" || fb.communityLinks[0].groupJID != "111@g.us" {
		t.Fatalf("links = %+v", fb.communityLinks)
	}
}

func TestCommunityUnlinkGroup_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/unlinkGroup/bot1", testKey, communityUnlinkGroupReq{CommunityJid: "120363@g.us"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no groupJid)", rec.Code)
	}
}

func TestCommunityLinkedGroups(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.linkedGroups = []wa.GroupLinkInfo{{JID: "111@g.us", Subject: "Grupo A"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/linkedGroups/bot1?communityJid=120363@g.us", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []communityLinkRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Subject != "Grupo A" || out[0].JID != "111@g.us" {
		t.Fatalf("linkedGroups = %+v", out)
	}
}

func TestCommunityLinkedGroups_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/linkedGroups/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityRequestList(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.communityReqs = []wa.CommunityMembershipRequest{{JID: "5512@s.whatsapp.net", Attrs: map[string]string{"t": "1"}}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/requestList/bot1?communityJid=120363@g.us", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []communityRequestRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].JID != "5512@s.whatsapp.net" || out[0].Attrs["t"] != "1" {
		t.Fatalf("requestList = %+v", out)
	}
}

func TestCommunityRequestUpdate(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/requestUpdate/bot1", testKey, communityRequestUpdateReq{
		CommunityJid: "120363@g.us", Participants: []string{"5512999"}, Action: "approve",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []participantResult
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Status != "200" {
		t.Fatalf("out = %+v", out)
	}
	if len(fb.communityReqUpdates) != 1 || fb.communityReqUpdates[0].action != "approve" || fb.communityReqUpdates[0].participants[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("reqUpdates = %+v", fb.communityReqUpdates)
	}
}

func TestCommunityRequestUpdate_BadAction(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/requestUpdate/bot1", testKey, communityRequestUpdateReq{
		CommunityJid: "120363@g.us", Participants: []string{"5512999"}, Action: "explode",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityRequestUpdate_NoParticipants(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/requestUpdate/bot1", testKey, communityRequestUpdateReq{
		CommunityJid: "120363@g.us", Action: "approve",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no participants)", rec.Code)
	}
}

func TestCommunityUpdateParticipant(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateParticipant/bot1", testKey, communityParticipantReq{
		CommunityJid: "120363@g.us", Participants: []string{"5512999"}, Action: "promote",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []participantResult
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Status != "200" {
		t.Fatalf("out = %+v", out)
	}
	if len(fb.communityPartUpdates) != 1 || fb.communityPartUpdates[0].action != "promote" || fb.communityPartUpdates[0].participants[0] != "5512999@s.whatsapp.net" {
		t.Fatalf("partUpdates = %+v", fb.communityPartUpdates)
	}
}

func TestCommunityUpdateParticipant_BadAction(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/updateParticipant/bot1", testKey, communityParticipantReq{
		CommunityJid: "120363@g.us", Participants: []string{"5512999"}, Action: "explode",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityJoinApprovalMode(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/joinApprovalMode/bot1", testKey, communityModeReq{CommunityJid: "120363@g.us", Mode: "on"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityModes) != 1 || fb.communityModes[0].kind != "join" || fb.communityModes[0].mode != "on" {
		t.Fatalf("modes = %+v", fb.communityModes)
	}
}

func TestCommunityJoinApprovalMode_BadMode(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/joinApprovalMode/bot1", testKey, communityModeReq{CommunityJid: "120363@g.us", Mode: "maybe"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityMemberAddMode(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/memberAddMode/bot1", testKey, communityModeReq{CommunityJid: "120363@g.us", Mode: "all_member_add"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityModes) != 1 || fb.communityModes[0].kind != "memberadd" || fb.communityModes[0].mode != "all_member_add" {
		t.Fatalf("modes = %+v", fb.communityModes)
	}
}

func TestCommunityMemberAddMode_BadMode(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/memberAddMode/bot1", testKey, communityModeReq{CommunityJid: "120363@g.us", Mode: "nobody"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunityToggleEphemeral(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/toggleEphemeral/bot1", testKey, communityToggleEphemeralReq{CommunityJid: "120363@g.us", Expiration: 604800})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityEphemerals) != 1 || fb.communityEphemerals[0].expiration != 604800 {
		t.Fatalf("ephemerals = %+v", fb.communityEphemerals)
	}
}

func TestCommunityToggleEphemeral_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/community/toggleEphemeral/bot1", testKey, communityToggleEphemeralReq{Expiration: 86400})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCommunitySettingUpdate(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	// action names must map to the lib's setting tags before reaching the backend,
	// identical to the group equivalent.
	for _, tc := range []struct{ action, want string }{
		{"announcement", "announce"},
		{"not_announcement", "not_announce"},
		{"locked", "locked"},
		{"unlocked", "unlocked"},
	} {
		rec := do(t, h, "POST", "/community/settingUpdate/bot1", testKey, communitySettingReq{CommunityJid: "120363@g.us", Action: tc.action})
		if rec.Code != http.StatusOK {
			t.Fatalf("action %q: status = %d; body=%s", tc.action, rec.Code, rec.Body.String())
		}
		if fb.lastCommunitySetting != tc.want {
			t.Fatalf("action %q passed %q to lib; want %q", tc.action, fb.lastCommunitySetting, tc.want)
		}
	}
	rec := do(t, h, "POST", "/community/settingUpdate/bot1", testKey, communitySettingReq{CommunityJid: "120363@g.us", Action: "nope"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action status = %d, want 400", rec.Code)
	}
}

func TestCommunityFetchAllParticipating(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.communityParticipating = []wa.GroupLinkInfo{{JID: "120363@g.us", Subject: "Bairro"}}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/fetchAllParticipating/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out []communityLinkRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].JID != "120363@g.us" || out[0].Subject != "Bairro" {
		t.Fatalf("participating = %+v", out)
	}
}

func TestCommunityFetchAllParticipating_Empty(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	// communityParticipating left nil — the handler must still serialize a JSON
	// array ([]) rather than null when there are no communities.
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/community/fetchAllParticipating/bot1", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 || rec.Body.Bytes()[0] != '[' {
		t.Fatalf("empty list must serialize as a JSON array, got %q", rec.Body.String())
	}
}

func TestCommunityLeave(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "DELETE", "/community/leave/bot1?communityJid=120363@g.us", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.communityLeaves) != 1 || fb.communityLeaves[0] != "120363@g.us" {
		t.Fatalf("leaves = %+v", fb.communityLeaves)
	}
}

func TestCommunityLeave_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "DELETE", "/community/leave/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
