package commands

import (
	"sync"
	"time"
)

const (
	DefaultDedupeTTL        = 24 * time.Hour
	DefaultDedupeMaxEntries = 1024
)

type DedupeStore struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	seen       map[string]time.Time
	now        func() time.Time
}

func NewDedupeStore(ttl time.Duration, maxEntries int) *DedupeStore {
	if ttl <= 0 {
		ttl = DefaultDedupeTTL
	}
	if maxEntries <= 0 {
		maxEntries = DefaultDedupeMaxEntries
	}
	return &DedupeStore{
		ttl:        ttl,
		maxEntries: maxEntries,
		seen:       make(map[string]time.Time),
		now:        time.Now,
	}
}

func (s *DedupeStore) Seen(commandID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	seenAt, ok := s.seen[commandID]
	if !ok {
		return false
	}
	if now.Sub(seenAt) > s.ttl {
		delete(s.seen, commandID)
		return false
	}
	return true
}

func (s *DedupeStore) Mark(commandID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(s.now().UTC())
	s.seen[commandID] = s.now().UTC()
	s.trimLocked()
}

func (s *DedupeStore) Prune() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
}

func (s *DedupeStore) Reserve(commandID string) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	s.pruneLocked(now)
	if _, ok := s.seen[commandID]; ok {
		return false
	}
	// Reserve 把“查重”和“标记”放在同一把锁里，避免并发 transport
	// 同时收到同一个 command_id 时通过 Seen/Mark 间隙重复执行。
	s.seen[commandID] = now
	s.trimLocked()
	return true
}

func (s *DedupeStore) pruneLocked(now time.Time) {
	for commandID, seenAt := range s.seen {
		if now.Sub(seenAt) > s.ttl {
			delete(s.seen, commandID)
		}
	}
}

func (s *DedupeStore) trimLocked() {
	for len(s.seen) > s.maxEntries {
		var oldestID string
		var oldestAt time.Time
		first := true
		for commandID, seenAt := range s.seen {
			if first || seenAt.Before(oldestAt) {
				oldestID = commandID
				oldestAt = seenAt
				first = false
			}
		}
		delete(s.seen, oldestID)
	}
}
