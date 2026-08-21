package savesync

// Custody changes as they happen, so a page showing who holds what does
// not have to poll to stay honest. One bus, many subscribers; the API
// layer turns a subscription into a server-sent event stream.
//
// Delivery is deliberately lossy: a subscriber whose buffer is full
// misses the notification rather than stalling the custody operation
// that produced it. That is safe because an event carries no state —
// it says "something about this world changed", and the client re-reads
// the truth from the API. A dropped event costs at most one refresh.

import (
	"sync"
	"time"
)

// Event names a world whose custody state changed.
type Event struct {
	WorldID int64     `json:"worldId"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
}

// Event kinds, matching the verbs that produce them.
const (
	EventCheckout   = "checkout"
	EventCheckin    = "checkin"
	EventCheckpoint = "checkpoint"
	EventClaim      = "claim"
	EventRelease    = "release"
	EventHead       = "head"
	EventImport     = "import"
	EventWorld      = "world" // created, settings changed, deleted
)

type subscriber struct {
	ch chan Event
}

// Subscribe returns a channel of custody events and the function that
// ends the subscription. The buffer absorbs a burst (a check-in that
// hands off to a claimant fires twice in a row); past it, events drop.
func (s *Service) Subscribe() (<-chan Event, func()) {
	sub := &subscriber{ch: make(chan Event, 16)}
	s.subMu.Lock()
	if s.subs == nil {
		s.subs = map[*subscriber]struct{}{}
	}
	s.subs[sub] = struct{}{}
	s.subMu.Unlock()

	return sub.ch, func() {
		s.subMu.Lock()
		if _, ok := s.subs[sub]; ok {
			delete(s.subs, sub)
			close(sub.ch)
		}
		s.subMu.Unlock()
	}
}

// publish fans an event out without blocking: custody correctness must
// never wait on a slow reader.
func (s *Service) publish(worldID int64, kind string) {
	ev := Event{WorldID: worldID, Kind: kind, At: time.Now()}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for sub := range s.subs {
		select {
		case sub.ch <- ev:
		default: // full: the reader will catch up on its next read anyway
		}
	}
}

// subscribers is the bus state, embedded in Service.
type subscribers struct {
	subMu sync.Mutex
	subs  map[*subscriber]struct{}
}
