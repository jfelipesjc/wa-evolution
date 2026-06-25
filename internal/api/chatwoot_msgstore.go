package api

import (
	"strconv"
	"sync"
)

// chatwoot_msgstore.go — the bidirectional WhatsApp<->Chatwoot message-id map that
// powers quoted/reply linkage (Evolution keeps this in its Message DB; we keep a
// bounded in-memory map per instance — quoting recent messages is the common case,
// and a restart degrades gracefully to "no quote" rather than breaking).

// waMsgRef is the minimal WhatsApp message identity needed to build a reply quote.
type waMsgRef struct {
	WAID        string
	RemoteJID   string
	FromMe      bool
	Text        string
	Participant string
}

// chatwootMsgStore maps, per instance, WhatsApp message id <-> Chatwoot message id
// (both directions), with a simple FIFO cap so it can't grow unbounded.
type chatwootMsgStore struct {
	mu       sync.Mutex
	cap      int
	waToCw   map[string]int      // key: instance|waID -> chatwoot message id
	cwToWA   map[string]waMsgRef // key: instance|chatwootID -> WA message ref
	order    []string            // insertion order of waToCw keys (for eviction)
	cwOrder  []string            // insertion order of cwToWA keys
}

func newChatwootMsgStore() *chatwootMsgStore {
	return &chatwootMsgStore{
		cap:    20000,
		waToCw: map[string]int{},
		cwToWA: map[string]waMsgRef{},
	}
}

func cwKey(instance string, id any) string {
	switch v := id.(type) {
	case string:
		return instance + "|" + v
	case int:
		return instance + "|" + strconv.Itoa(v)
	}
	return instance
}

// record stores both directions for a bridged message: the WA id, the Chatwoot
// message id, and the WA message ref (so it can later be quoted on either side).
func (s *chatwootMsgStore) record(instance string, chatwootID int, ref waMsgRef) {
	if ref.WAID == "" || chatwootID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	wk := cwKey(instance, ref.WAID)
	ck := cwKey(instance, chatwootID)
	if _, ok := s.waToCw[wk]; !ok {
		s.order = append(s.order, wk)
	}
	s.waToCw[wk] = chatwootID
	if _, ok := s.cwToWA[ck]; !ok {
		s.cwOrder = append(s.cwOrder, ck)
	}
	s.cwToWA[ck] = ref
	s.evict()
}

func (s *chatwootMsgStore) evict() {
	for len(s.order) > s.cap {
		k := s.order[0]
		s.order = s.order[1:]
		delete(s.waToCw, k)
	}
	for len(s.cwOrder) > s.cap {
		k := s.cwOrder[0]
		s.cwOrder = s.cwOrder[1:]
		delete(s.cwToWA, k)
	}
}

// chatwootIDForWA returns the Chatwoot message id for a WhatsApp message id (used
// to set in_reply_to when a received message quotes an earlier one).
func (s *chatwootMsgStore) chatwootIDForWA(instance, waID string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.waToCw[cwKey(instance, waID)]
	return id, ok
}

// waRefForChatwootID returns the WhatsApp message ref for a Chatwoot message id
// (used to build a WhatsApp reply quote when the agent replies to a message).
func (s *chatwootMsgStore) waRefForChatwootID(instance string, chatwootID int) (waMsgRef, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.cwToWA[cwKey(instance, chatwootID)]
	return r, ok
}
