package api

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGetNumbers(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		// 14-char (with 9): also produce the 13-char (drop the 9 at index 5).
		{"+5512981201631", []string{"+5512981201631", "+551281201631"}},
		// 13-char (without 9): also produce the 14-char (insert 9 at index 5).
		{"+551281201631", []string{"+551281201631", "+5512981201631"}},
		// non-BR: single element.
		{"+14155551234", []string{"+14155551234"}},
		// BR but wrong length: single element.
		{"+5512", []string{"+5512"}},
	}
	for _, c := range cases {
		got := getNumbers(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("getNumbers(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestGetFilterPayload(t *testing.T) {
	p := getFilterPayload("+5512981201631")
	if len(p) != 2 {
		t.Fatalf("len = %d, want 2", len(p))
	}
	// values stripped of '+'.
	if p[0].Values[0] != "5512981201631" || p[1].Values[0] != "551281201631" {
		t.Fatalf("values = %v / %v", p[0].Values, p[1].Values)
	}
	if p[0].QueryOperator == nil || *p[0].QueryOperator != "OR" {
		t.Fatalf("first query_operator = %v, want OR", p[0].QueryOperator)
	}
	if p[1].QueryOperator != nil {
		t.Fatalf("last query_operator = %v, want null", p[1].QueryOperator)
	}
}

func TestFindContact_Single(t *testing.T) {
	mock := newFullMock()
	mock.contacts = []cwContact{{ID: 201, Identifier: "5512981201631@s.whatsapp.net", PhoneNumber: "+5512981201631"}}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t", MergeBrazilContacts: true})

	c, err := cw.FindContact(context.Background(), "5512981201631")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || c.ID != 201 {
		t.Fatalf("found = %#v", c)
	}
}

func TestFindContact_BrazilMerge(t *testing.T) {
	mock := newFullMock()
	// Two Brazilian duplicates: with-9 (14-char phone) and without-9 (13-char).
	mock.contacts = []cwContact{
		{ID: 10, PhoneNumber: "+5512981201631"}, // 14 chars -> survivor
		{ID: 11, PhoneNumber: "+551281201631"},  // 13 chars -> absorbed
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t", MergeBrazilContacts: true})

	c, err := cw.FindContact(context.Background(), "5512981201631")
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.mergeReqs) != 1 {
		t.Fatalf("merge calls = %d, want 1", len(mock.mergeReqs))
	}
	if mock.mergeReqs[0]["base_contact_id"] != 10 {
		t.Fatalf("base_contact_id (survivor) = %d, want 10 (14-char)", mock.mergeReqs[0]["base_contact_id"])
	}
	if mock.mergeReqs[0]["mergee_contact_id"] != 11 {
		t.Fatalf("mergee_contact_id = %d, want 11 (13-char)", mock.mergeReqs[0]["mergee_contact_id"])
	}
	if c == nil || c.ID != 10 {
		t.Fatalf("survivor returned = %#v", c)
	}
}

func TestFindContact_MergeDisabledLongestWins(t *testing.T) {
	mock := newFullMock()
	mock.contacts = []cwContact{
		{ID: 10, PhoneNumber: "+5512981201631"},
		{ID: 11, PhoneNumber: "+551281201631"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t", MergeBrazilContacts: false})

	c, err := cw.FindContact(context.Background(), "5512981201631")
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.mergeReqs) != 0 {
		t.Fatalf("merge fired with mergeBrazilContacts=false")
	}
	// longest variant (the with-9, 14-char) wins.
	if c == nil || c.ID != 10 {
		t.Fatalf("longest-variant pick = %#v, want id 10", c)
	}
}

func TestCreateContact(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t"})

	c, err := cw.CreateContact(context.Background(), 50, "Felipe", "5512981201631@s.whatsapp.net", "")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || c.ID == 0 {
		t.Fatalf("created = %#v", c)
	}
	req := mock.createReqs[0]
	if req["phone_number"] != "+5512981201631" {
		t.Fatalf("phone_number = %v", req["phone_number"])
	}
	if req["identifier"] != "5512981201631@s.whatsapp.net" {
		t.Fatalf("identifier = %v", req["identifier"])
	}
	// inbox_id sent as a number.
	if _, ok := req["inbox_id"].(float64); !ok {
		t.Fatalf("inbox_id not a number: %#v", req["inbox_id"])
	}
}

func TestCreateContact_422FallsBackToIdentifier(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	jid := "5512981201631@s.whatsapp.net"
	mock.create422jid = jid
	// existing contact owning that identifier.
	mock.contacts = []cwContact{{ID: 99, Identifier: jid, PhoneNumber: "+5512981201631"}}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t"})

	c, err := cw.CreateContact(context.Background(), 50, "Felipe", jid, "")
	if err != nil {
		t.Fatalf("expected 422 fallback, got err: %v", err)
	}
	if c == nil || c.ID != 99 {
		t.Fatalf("fallback contact = %#v, want id 99", c)
	}
}

func TestCreateConversation_StringIDs(t *testing.T) {
	mock := newFullMock()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t"})

	id, err := cw.CreateConversation(context.Background(), 201, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatalf("conversation id = 0")
	}
	body := mock.createdConvs[0]
	if body["contact_id"] != "201" || body["inbox_id"] != "50" {
		t.Fatalf("ids not strings: %#v", body)
	}
	if _, ok := body["status"]; ok {
		t.Fatalf("status set when not pending: %#v", body)
	}
}

func TestCreateConversation_Pending(t *testing.T) {
	mock := newFullMock()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t"})
	if _, err := cw.CreateConversation(context.Background(), 201, 50, true); err != nil {
		t.Fatal(err)
	}
	if mock.createdConvs[0]["status"] != "pending" {
		t.Fatalf("status = %v, want pending", mock.createdConvs[0]["status"])
	}
}

func TestCreateTextMessage(t *testing.T) {
	mock := newFullMock()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()
	cw := newChatwootClient(chatwootConfig{URL: srv.URL, AccountID: "1", Token: "t"})
	id, err := cw.CreateTextMessage(context.Background(), 300, "hi", "incoming", "WAID:abc", 0)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatalf("message id = 0")
	}
	b := mock.messages[0]
	if b["content"] != "hi" || b["message_type"] != "incoming" || b["source_id"] != "WAID:abc" || b["private"] != false {
		t.Fatalf("message body = %#v", b)
	}
}

func TestNormalizeJid(t *testing.T) {
	cases := map[string]string{
		"5512981201631@s.whatsapp.net":    "5512981201631",
		"5512981201631:12@s.whatsapp.net": "5512981201631",
		"123456@lid":                      "123456@lid",
		"":                                "",
	}
	for in, want := range cases {
		if got := normalizeJid(in); got != want {
			t.Errorf("normalizeJid(%q) = %q, want %q", in, got, want)
		}
	}
}
