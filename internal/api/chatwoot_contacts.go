package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// chatwoot_contacts.go — Phase 2a: Chatwoot contact lookup/create/merge with the
// Brazilian 9th-digit duplicate-merge algorithm. Mirrors Evolution's
// chatwoot.service.ts contact cluster (findContact / getNumbers / getFilterPayload
// / mergeBrazilianContacts / createContact / findContactByIdentifier /
// updateContact / mergeContacts). All paths are /api/v1/accounts/{accountId}/...

// cwContact is the subset of a Chatwoot contact we read.
type cwContact struct {
	ID          int    `json:"id"`
	Identifier  string `json:"identifier"`
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name"`
}

// getNumbers expands a +-prefixed phone string into the original plus, for
// Brazilian (+55) numbers of length 13/14, the 9th-digit variant. Original first.
//
//	+5512981201631 (14) -> [+5512981201631, +551281201631]   (drop '9' at index 5)
//	+551281201631  (13) -> [+551281201631, +5512981201631]   (insert '9' at index 5)
func getNumbers(query string) []string {
	numbers := []string{query}
	if strings.HasPrefix(query, "+55") && len(query) == 14 {
		withoutNine := query[0:5] + query[6:]
		numbers = append(numbers, withoutNine)
	} else if strings.HasPrefix(query, "+55") && len(query) == 13 {
		withNine := query[0:5] + "9" + query[5:]
		numbers = append(numbers, withNine)
	}
	return numbers
}

// cwFilterClause is one entry in a Chatwoot /contacts/filter OR-chain.
type cwFilterClause struct {
	AttributeKey   string   `json:"attribute_key"`
	FilterOperator string   `json:"filter_operator"`
	Values         []string `json:"values"`
	QueryOperator  *string  `json:"query_operator"`
}

// getFilterPayload builds the OR-chain over ["phone_number"] x getNumbers(query).
// Values are stripped of '+'; every clause is "OR" except the last (null).
func getFilterPayload(query string) []cwFilterClause {
	numbers := getNumbers(query)
	fields := []string{"phone_number"}
	or := "OR"
	var payload []cwFilterClause
	for i1, field := range fields {
		for i2, number := range numbers {
			var op *string
			if i1 == len(fields)-1 && i2 == len(numbers)-1 {
				op = nil
			} else {
				op = &or
			}
			payload = append(payload, cwFilterClause{
				AttributeKey:   field,
				FilterOperator: "equal_to",
				Values:         []string{strings.ReplaceAll(number, "+", "")},
				QueryOperator:  op,
			})
		}
	}
	return payload
}

// FindContact searches Chatwoot for a contact by phone digits (no '@'). On >1
// result it runs findContactInContactList (Brazilian merge or longest-variant).
// Returns nil (no error) when nothing matches.
func (cw *chatwootClient) FindContact(ctx context.Context, phoneDigits string) (*cwContact, error) {
	query := "+" + phoneDigits
	body := map[string]any{"payload": getFilterPayload(query)}
	var resp struct {
		Payload []cwContact `json:"payload"`
	}
	if err := cw.do(ctx, http.MethodPost, cw.acctPath("/contacts/filter"), body, &resp); err != nil {
		return nil, err
	}
	switch {
	case len(resp.Payload) == 0:
		return nil, nil
	case len(resp.Payload) == 1:
		c := resp.Payload[0]
		return &c, nil
	default:
		return cw.findContactInContactList(ctx, resp.Payload, query)
	}
}

// findContactInContactList resolves a multi-match: when exactly two Brazilian
// duplicates and merge is enabled, merge them (14-char survives). Otherwise pick
// the contact whose phone_number equals the longest variant, else any variant.
func (cw *chatwootClient) findContactInContactList(ctx context.Context, contacts []cwContact, query string) (*cwContact, error) {
	phoneNumbers := getNumbers(query)

	if len(contacts) == 2 && cw.mergeBrazilContacts && strings.HasPrefix(query, "+55") {
		if merged := cw.mergeBrazilianContacts(ctx, contacts); merged != nil {
			return merged, nil
		}
		// merge failed -> fall through to longest-variant pick
	}

	// longest variant (the with-9 number)
	longest := ""
	for _, n := range phoneNumbers {
		if len(n) > len(longest) {
			longest = n
		}
	}
	for i := range contacts {
		if contacts[i].PhoneNumber == longest {
			return &contacts[i], nil
		}
	}

	// last resort: any contact whose phone_number matches a variant
	for i := range contacts {
		if contacts[i].PhoneNumber == "" {
			continue
		}
		for _, n := range phoneNumbers {
			if contacts[i].PhoneNumber == n {
				return &contacts[i], nil
			}
		}
	}
	return nil, nil
}

// mergeBrazilianContacts merges the with-9 (14-char phone) survivor and the
// without-9 (13-char) mergee via /actions/contact_merge. Returns nil on failure
// (caller falls through to the longest-variant pick).
func (cw *chatwootClient) mergeBrazilianContacts(ctx context.Context, contacts []cwContact) *cwContact {
	var baseID, mergeeID int
	var base *cwContact
	for i := range contacts {
		if len(contacts[i].PhoneNumber) == 14 {
			baseID = contacts[i].ID
			base = &contacts[i]
		}
		if len(contacts[i].PhoneNumber) == 13 {
			mergeeID = contacts[i].ID
		}
	}
	body := map[string]any{"base_contact_id": baseID, "mergee_contact_id": mergeeID}
	var out cwContact
	if err := cw.do(ctx, http.MethodPost, cw.acctPath("/actions/contact_merge"), body, &out); err != nil {
		return nil
	}
	if out.ID != 0 {
		return &out
	}
	return base
}

// CreateContact creates a Chatwoot contact. phone_number ("+digits") is set only
// when jid contains '@' or jid is absent; identifier is the full jid. Handles both
// response shapes (id at top or payload.contact.id). On 422 with a jid it falls
// back to FindContactByIdentifier.
func (cw *chatwootClient) CreateContact(ctx context.Context, inboxID int, name, jid, avatarURL string) (*cwContact, error) {
	phoneDigits := strings.SplitN(jid, "@", 2)[0]
	if name == "" {
		name = phoneDigits
	}
	body := map[string]any{
		"inbox_id":   inboxID,
		"name":       name,
		"identifier": jid,
	}
	if avatarURL != "" {
		body["avatar_url"] = avatarURL
	}
	if strings.Contains(jid, "@") || jid == "" {
		body["phone_number"] = "+" + phoneDigits
	}

	var resp struct {
		ID      int    `json:"id"`
		Payload struct {
			Contact cwContact `json:"contact"`
		} `json:"payload"`
		// also handle a bare contact body
		Identifier  string `json:"identifier"`
		PhoneNumber string `json:"phone_number"`
		Name        string `json:"name"`
	}
	err := cw.do(ctx, http.MethodPost, cw.acctPath("/contacts"), body, &resp)
	if err != nil {
		if jid != "" && strings.Contains(err.Error(), " 422 ") {
			return cw.FindContactByIdentifier(ctx, jid)
		}
		return nil, err
	}
	if resp.Payload.Contact.ID != 0 {
		c := resp.Payload.Contact
		return &c, nil
	}
	return &cwContact{
		ID:          resp.ID,
		Identifier:  resp.Identifier,
		PhoneNumber: resp.PhoneNumber,
		Name:        resp.Name,
	}, nil
}

// FindContactByIdentifier looks up a contact by its identifier: first the search
// endpoint, then a filter on the identifier attribute. Returns nil when absent.
func (cw *chatwootClient) FindContactByIdentifier(ctx context.Context, identifier string) (*cwContact, error) {
	// Step 1: search?q=&sort=name
	searchURL := cw.acctPath("/contacts/search") + "?q=" + url.QueryEscape(identifier) + "&sort=name"
	var sresp struct {
		Payload []cwContact `json:"payload"`
	}
	if err := cw.do(ctx, http.MethodGet, searchURL, nil, &sresp); err == nil {
		if len(sresp.Payload) > 0 {
			c := sresp.Payload[0]
			return &c, nil
		}
	}

	// Step 2: filter by identifier attribute
	body := map[string]any{"payload": []cwFilterClause{{
		AttributeKey:   "identifier",
		FilterOperator: "equal_to",
		Values:         []string{identifier},
		QueryOperator:  nil,
	}}}
	var fresp struct {
		Payload []cwContact `json:"payload"`
	}
	if err := cw.do(ctx, http.MethodPost, cw.acctPath("/contacts/filter"), body, &fresp); err != nil {
		return nil, err
	}
	if len(fresp.Payload) > 0 {
		c := fresp.Payload[0]
		return &c, nil
	}
	return nil, nil
}

// UpdateContact applies a partial update (PUT /contacts/{id}). Returns nil (NOT an
// error) on failure — the null return is a signal to callers that the update
// failed (likely an identifier conflict) and they should merge instead.
func (cw *chatwootClient) UpdateContact(ctx context.Context, id int, fields map[string]any) *cwContact {
	var resp struct {
		ID      int    `json:"id"`
		Payload struct {
			Contact cwContact `json:"contact"`
		} `json:"payload"`
		Identifier  string `json:"identifier"`
		PhoneNumber string `json:"phone_number"`
		Name        string `json:"name"`
	}
	if err := cw.do(ctx, http.MethodPut, cw.acctPath(fmt.Sprintf("/contacts/%d", id)), fields, &resp); err != nil {
		return nil
	}
	if resp.Payload.Contact.ID != 0 {
		c := resp.Payload.Contact
		return &c
	}
	if resp.ID == 0 {
		resp.ID = id
	}
	return &cwContact{
		ID:          resp.ID,
		Identifier:  resp.Identifier,
		PhoneNumber: resp.PhoneNumber,
		Name:        resp.Name,
	}
}

// MergeContacts merges mergeeID into baseID via /actions/contact_merge.
func (cw *chatwootClient) MergeContacts(ctx context.Context, baseID, mergeeID int) error {
	body := map[string]any{"base_contact_id": baseID, "mergee_contact_id": mergeeID}
	return cw.do(ctx, http.MethodPost, cw.acctPath("/actions/contact_merge"), body, nil)
}
