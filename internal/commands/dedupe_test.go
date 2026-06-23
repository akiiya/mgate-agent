package commands

import (
	"testing"
	"time"
)

func TestDedupeStoreSeenMarkAndTTL(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	store := NewDedupeStore(time.Minute, 10)
	store.now = func() time.Time { return now }

	if store.Seen("cmd_001") {
		t.Fatal("command should not be seen before Mark")
	}
	store.Mark("cmd_001")
	if !store.Seen("cmd_001") {
		t.Fatal("command should be seen after Mark")
	}

	now = now.Add(2 * time.Minute)
	if store.Seen("cmd_001") {
		t.Fatal("command should expire after TTL")
	}
}

func TestDedupeStoreReserveIsAtomic(t *testing.T) {
	store := NewDedupeStore(time.Hour, 10)

	if !store.Reserve("cmd_001") {
		t.Fatal("first Reserve() should pass")
	}
	if store.Reserve("cmd_001") {
		t.Fatal("second Reserve() should fail")
	}
}

func TestDedupeStoreTrimsOldEntries(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	store := NewDedupeStore(time.Hour, 2)
	store.now = func() time.Time { return now }

	store.Mark("cmd_001")
	now = now.Add(time.Second)
	store.Mark("cmd_002")
	now = now.Add(time.Second)
	store.Mark("cmd_003")

	if store.Seen("cmd_001") {
		t.Fatal("oldest command should be trimmed")
	}
	if !store.Seen("cmd_002") || !store.Seen("cmd_003") {
		t.Fatal("newer commands should remain")
	}
}
