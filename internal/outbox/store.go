package outbox

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
)

const (
	DefaultMaxRecords = 100
	DefaultMaxBytes   = 5 * 1024 * 1024
)

type Store struct {
	dir        string
	maxRecords int
	maxBytes   int64
	audit      *audit.Writer
	mu         sync.Mutex
}

type Options struct {
	Dir        string
	MaxRecords int
	MaxBytes   int64
	Audit      *audit.Writer
}

func NewStore(opts Options) (*Store, error) {
	if strings.TrimSpace(opts.Dir) == "" {
		return nil, errors.New("outbox dir is required")
	}
	if opts.MaxRecords <= 0 {
		opts.MaxRecords = DefaultMaxRecords
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	store := &Store{
		dir:        opts.Dir,
		maxRecords: opts.MaxRecords,
		maxBytes:   opts.MaxBytes,
		audit:      opts.Audit,
	}
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create outbox dir: %w", err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read outbox dir: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(s.dir, name)
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			_ = os.Remove(path)
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := s.readRecordFile(path); err != nil {
			badPath := path + ".bad"
			_ = os.Rename(path, badPath)
			s.writeAudit(audit.Event{
				Event:     audit.EventOutboxRecordCorrupted,
				RecordID:  strings.TrimSuffix(name, ".json"),
				ErrorCode: "corrupted_record",
			})
		}
	}
	return nil
}

func (s *Store) Save(env protocol.Envelope) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	record, err := NewRecord(env, now)
	if err != nil {
		return Record{}, err
	}
	if old, err := s.readRecordFile(s.path(record.RecordID)); err == nil {
		record.CreatedAt = old.CreatedAt
		record.Attempts = old.Attempts
		record.LastError = old.LastError
		record.NextRetryAt = old.NextRetryAt
	}
	if err := s.writeRecordAtomic(record); err != nil {
		return Record{}, err
	}
	s.writeAudit(audit.Event{
		Event:     audit.EventOutboxRecordSaved,
		RecordID:  record.RecordID,
		CommandID: record.CommandID,
		Attempts:  record.Attempts,
	})
	if err := s.enforceLimitsLocked(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) Due(now time.Time, limit int) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.listLocked()
	if err != nil {
		return nil, err
	}
	var due []Record
	for _, record := range records {
		if record.NextRetryAt.IsZero() || !record.NextRetryAt.After(now.UTC()) {
			due = append(due, record)
			if limit > 0 && len(due) >= limit {
				break
			}
		}
	}
	return due, nil
}

func (s *Store) All() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) MarkFailure(recordID string, sendErr error) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.readRecordFile(s.path(recordID))
	if err != nil {
		return Record{}, err
	}
	record.Attempts++
	record.UpdatedAt = time.Now().UTC()
	record.LastError = shortError(sendErr)
	delay := retryDelay(record.Attempts)
	record.NextRetryAt = record.UpdatedAt.Add(delay + retryJitter(delay))
	if err := s.writeRecordAtomic(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) Delete(recordID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, _ := s.readRecordFile(s.path(recordID))
	if err := os.Remove(s.path(recordID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.writeAudit(audit.Event{
		Event:     audit.EventOutboxRecordDeleted,
		RecordID:  recordID,
		CommandID: record.CommandID,
	})
	return nil
}

func (s *Store) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.listLocked()
	if err != nil {
		return 0
	}
	return len(records)
}

func (s *Store) path(recordID string) string {
	return filepath.Join(s.dir, recordID+".json")
}

func (s *Store) writeRecordAtomic(record Record) error {
	// outbox 是结果可靠性的边界，必须先写临时文件并 rename，避免半截 JSON 被当成有效记录。
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path(record.RecordID) + ".tmp"
	finalPath := s.path(record.RecordID)
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}
	s.syncDir()
	return nil
}

func (s *Store) readRecordFile(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	if record.RecordID == "" || record.CommandID == "" || record.Envelope.Type != protocol.MessageTypeResult {
		return Record{}, errors.New("invalid outbox record")
	}
	return record, nil
}

func (s *Store) listLocked() ([]Record, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.readRecordFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (s *Store) enforceLimitsLocked() error {
	for {
		records, totalBytes, err := s.snapshotLocked()
		if err != nil {
			return err
		}
		if len(records) <= s.maxRecords && totalBytes <= s.maxBytes {
			return nil
		}
		if len(records) == 0 {
			return nil
		}
		drop := records[0]
		if err := os.Remove(s.path(drop.RecordID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		s.writeAudit(audit.Event{
			Event:     audit.EventOutboxRecordDropped,
			RecordID:  drop.RecordID,
			CommandID: drop.CommandID,
			Attempts:  drop.Attempts,
			ErrorCode: "outbox_limit",
		})
	}
}

func (s *Store) snapshotLocked() ([]Record, int64, error) {
	records, err := s.listLocked()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	for _, record := range records {
		info, err := os.Stat(s.path(record.RecordID))
		if err == nil {
			total += info.Size()
		}
	}
	return records, total, nil
}

func (s *Store) syncDir() {
	dir, err := os.Open(s.dir)
	if err != nil {
		return
	}
	defer dir.Close()
	_ = dir.Sync()
}

func (s *Store) writeAudit(event audit.Event) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Write(event)
}

func retryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return 5 * time.Second
	}
	delay := 5 * time.Second
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= time.Minute {
			return time.Minute
		}
	}
	return delay
}

func retryJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	spread := delay / 5
	if spread <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(spread*2+1)))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64()) - spread
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 200 {
		return msg[:200]
	}
	return msg
}
