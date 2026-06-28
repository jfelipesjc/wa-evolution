package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	wa "github.com/jfelipesjc/wa-go/wa"
)

func TestNewsletterFind(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.newsletterInfo = &wa.NewsletterInfo{
		JID: "1234@newsletter", Name: "Canal", Description: "desc", Invite: "abc",
		SubscriberCount: 42, Verification: "verified", CreationTime: 100, MuteState: "off",
	}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/findNewsletter/bot1?key=1234@newsletter", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var nr newsletterResp
	_ = json.Unmarshal(rec.Body.Bytes(), &nr)
	if nr.Name != "Canal" || nr.SubscriberCount != 42 || nr.Invite != "abc" || nr.MuteState != "off" {
		t.Fatalf("resp = %+v", nr)
	}
}

func TestNewsletterFind_MissingKey(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/findNewsletter/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterUnfollow(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/unfollow/bot1", testKey, newsletterJidReq{JID: "1234@newsletter"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterUnfollows) != 1 || fb.newsletterUnfollows[0] != "1234@newsletter" {
		t.Fatalf("unfollows = %+v", fb.newsletterUnfollows)
	}
}

func TestNewsletterUnfollow_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/unfollow/bot1", testKey, newsletterJidReq{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterMuteAndUnmute(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/mute/bot1", testKey, newsletterJidReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusOK {
		t.Fatalf("mute status = %d; body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/newsletter/unmute/bot1", testKey, newsletterJidReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unmute status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterMutes) != 2 || !fb.newsletterMutes[0].mute || fb.newsletterMutes[1].mute {
		t.Fatalf("mutes = %+v", fb.newsletterMutes)
	}
}

func TestNewsletterMute_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/mute/bot1", testKey, newsletterJidReq{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterUpdateName(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updateName/bot1", testKey, newsletterUpdateNameReq{NewsletterJid: "1234@newsletter", Name: "Novo"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterUpdates) != 1 || fb.newsletterUpdates[0].field != "name" || fb.newsletterUpdates[0].value != "Novo" {
		t.Fatalf("updates = %+v", fb.newsletterUpdates)
	}
}

func TestNewsletterUpdateName_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updateName/bot1", testKey, newsletterUpdateNameReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no name)", rec.Code)
	}
}

func TestNewsletterUpdateDescription(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updateDescription/bot1", testKey, newsletterUpdateDescriptionReq{NewsletterJid: "1234@newsletter", Description: "nova desc"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterUpdates) != 1 || fb.newsletterUpdates[0].field != "description" || fb.newsletterUpdates[0].value != "nova desc" {
		t.Fatalf("updates = %+v", fb.newsletterUpdates)
	}
}

func TestNewsletterUpdateDescription_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updateDescription/bot1", testKey, newsletterUpdateDescriptionReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no description)", rec.Code)
	}
}

func TestNewsletterUpdatePicture(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updatePicture/bot1", testKey, newsletterUpdatePictureReq{NewsletterJid: "1234@newsletter", Picture: "AAAA"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterUpdates) != 1 || fb.newsletterUpdates[0].field != "picture" || fb.newsletterUpdates[0].value != "AAAA" {
		t.Fatalf("updates = %+v", fb.newsletterUpdates)
	}
}

func TestNewsletterUpdatePicture_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updatePicture/bot1", testKey, newsletterUpdatePictureReq{Picture: "AAAA"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no newsletterJid)", rec.Code)
	}
}

// An empty picture removes the channel photo, so it must be accepted (200) and
// reach the backend with picture=="".
func TestNewsletterUpdatePicture_EmptyRemoves(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/updatePicture/bot1", testKey, newsletterUpdatePictureReq{NewsletterJid: "1234@newsletter", Picture: ""})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty picture removes); body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterUpdates) != 1 || fb.newsletterUpdates[0].field != "picture" || fb.newsletterUpdates[0].value != "" {
		t.Fatalf("updates = %+v, want one picture update with empty value", fb.newsletterUpdates)
	}
}

func TestNewsletterReactionMode(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactionMode/bot1", testKey, newsletterReactionModeReq{NewsletterJid: "1234@newsletter", Mode: "basic"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if fb.lastNewsletterReactionMode != "BASIC" {
		t.Fatalf("reaction mode = %q, want BASIC (upper-cased)", fb.lastNewsletterReactionMode)
	}
}

func TestNewsletterReactionMode_Invalid(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactionMode/bot1", testKey, newsletterReactionModeReq{NewsletterJid: "1234@newsletter", Mode: "loud"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (bad mode)", rec.Code)
	}
}

func TestNewsletterReactionMode_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactionMode/bot1", testKey, newsletterReactionModeReq{Mode: "ALL"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no jid)", rec.Code)
	}
}

func TestNewsletterFetchMessages(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.newsletterMsgs = []wa.NewsletterMessage{
		{ServerID: "100", Timestamp: 111, Type: "text", Content: []byte("hi")},
	}
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/fetchMessages/bot1?newsletterJid=1234@newsletter&count=10", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var msgs []newsletterMessageRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs) != 1 || msgs[0].ServerID != "100" || msgs[0].Timestamp != 111 {
		t.Fatalf("msgs = %+v", msgs)
	}
	if msgs[0].Content != base64.StdEncoding.EncodeToString([]byte("hi")) {
		t.Fatalf("content = %q", msgs[0].Content)
	}
	// The parsed count must reach the backend.
	if len(fb.newsletterFetches) != 1 || fb.newsletterFetches[0].count != 10 {
		t.Fatalf("fetch recorded %+v, want count=10", fb.newsletterFetches)
	}
}

func TestNewsletterFetchMessages_DefaultCount(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	// No count query param -> the handler default (50) must reach the backend.
	rec := do(t, h, "GET", "/newsletter/fetchMessages/bot1?newsletterJid=1234@newsletter", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterFetches) != 1 || fb.newsletterFetches[0].count != 50 {
		t.Fatalf("fetch recorded %+v, want default count=50", fb.newsletterFetches)
	}
}

func TestNewsletterFetchMessages_NonPositiveCountKeepsDefault(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	// count<=0 is ignored and the default (50) is kept.
	rec := do(t, h, "GET", "/newsletter/fetchMessages/bot1?newsletterJid=1234@newsletter&count=0", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterFetches) != 1 || fb.newsletterFetches[0].count != 50 {
		t.Fatalf("fetch recorded %+v, want count=50 (non-positive ignored)", fb.newsletterFetches)
	}
}

func TestNewsletterFetchMessages_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/fetchMessages/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterAdminCount(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.newsletterAdminCnt = 3
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/adminCount/bot1?newsletterJid=1234@newsletter", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["adminCount"] != 3 {
		t.Fatalf("adminCount = %d, want 3", out["adminCount"])
	}
}

func TestNewsletterAdminCount_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/adminCount/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterChangeOwner(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/changeOwner/bot1", testKey, newsletterUserReq{NewsletterJid: "1234@newsletter", UserJid: "5512999"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterOwnerChanges) != 1 || fb.newsletterOwnerChanges[0].userJid != "5512999@s.whatsapp.net" {
		t.Fatalf("ownerChanges = %+v (userJid should be normalized)", fb.newsletterOwnerChanges)
	}
}

func TestNewsletterChangeOwner_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/changeOwner/bot1", testKey, newsletterUserReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no userJid)", rec.Code)
	}
}

func TestNewsletterDemote(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/demote/bot1", testKey, newsletterUserReq{NewsletterJid: "1234@newsletter", UserJid: "5512888"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterDemotes) != 1 || fb.newsletterDemotes[0].userJid != "5512888@s.whatsapp.net" {
		t.Fatalf("demotes = %+v", fb.newsletterDemotes)
	}
}

func TestNewsletterDemote_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/demote/bot1", testKey, newsletterUserReq{UserJid: "5512888"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no newsletterJid)", rec.Code)
	}
}

func TestNewsletterSubscribe(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.newsletterSubDuration = "300"
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/subscribeUpdates/bot1", testKey, newsletterJidReq{NewsletterJid: "1234@newsletter"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["duration"] != "300" {
		t.Fatalf("duration = %q, want 300", out["duration"])
	}
	if len(fb.newsletterSubscribes) != 1 || fb.newsletterSubscribes[0] != "1234@newsletter" {
		t.Fatalf("subscribes = %+v", fb.newsletterSubscribes)
	}
}

func TestNewsletterSubscribe_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/subscribeUpdates/bot1", testKey, newsletterJidReq{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterDelete(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "DELETE", "/newsletter/delete/bot1?newsletterJid=1234@newsletter", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterDeletes) != 1 || fb.newsletterDeletes[0] != "1234@newsletter" {
		t.Fatalf("deletes = %+v", fb.newsletterDeletes)
	}
}

func TestNewsletterDelete_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "DELETE", "/newsletter/delete/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterDelete_BodyFallback(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	// DELETE with no query param but a JSON body carrying the JID (documented fallback).
	rec := do(t, h, "DELETE", "/newsletter/delete/bot1", testKey, newsletterJidReq{NewsletterJid: "5678@newsletter"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterDeletes) != 1 || fb.newsletterDeletes[0] != "5678@newsletter" {
		t.Fatalf("deletes = %+v", fb.newsletterDeletes)
	}
}

func TestNewsletterSubscribers(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	fb.newsletterSubCount = 128
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/subscribers/bot1?newsletterJid=1234@newsletter", testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["subscribers"] != 128 {
		t.Fatalf("subscribers = %d, want 128", out["subscribers"])
	}
}

func TestNewsletterSubscribers_MissingJID(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "GET", "/newsletter/subscribers/bot1", testKey, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNewsletterReactMessage(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactMessage/bot1", testKey, newsletterReactReq{NewsletterJid: "1234@newsletter", ServerID: "100", Reaction: "👍"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterReacts) != 1 || fb.newsletterReacts[0].serverID != "100" || fb.newsletterReacts[0].reaction != "👍" {
		t.Fatalf("reacts = %+v", fb.newsletterReacts)
	}
}

// An empty reaction removes the reaction, so it must be accepted (200) and reach
// the backend with reaction=="".
func TestNewsletterReactMessage_EmptyRemoves(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactMessage/bot1", testKey, newsletterReactReq{NewsletterJid: "1234@newsletter", ServerID: "100", Reaction: ""})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty reaction removes); body=%s", rec.Code, rec.Body.String())
	}
	if len(fb.newsletterReacts) != 1 || fb.newsletterReacts[0].reaction != "" {
		t.Fatalf("reacts = %+v, want one react with empty reaction", fb.newsletterReacts)
	}
}

func TestNewsletterReactMessage_Validation(t *testing.T) {
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	h := newTestServer(t, fb)
	rec := do(t, h, "POST", "/newsletter/reactMessage/bot1", testKey, newsletterReactReq{ServerID: "100", Reaction: "👍"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no newsletterJid)", rec.Code)
	}
	rec = do(t, h, "POST", "/newsletter/reactMessage/bot1", testKey, newsletterReactReq{NewsletterJid: "1234@newsletter", Reaction: "👍"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no serverId)", rec.Code)
	}
}
