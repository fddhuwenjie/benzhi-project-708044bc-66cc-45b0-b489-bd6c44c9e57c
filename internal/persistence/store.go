package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"seedvault/internal/audit"
	"seedvault/internal/domain"
	"sync"
)

// ErrSnapshotCorrupt is the sentinel returned by a Store whose on-disk
// snapshots could not be fully loaded. The Store is left in a read-only
// degenerate state: reads reflect whatever could be decoded (which may be
// partial) while every write is rejected so the damaged snapshot files are
// never overwritten by partially loaded state.
var ErrSnapshotCorrupt = errors.New("snapshot_corrupt")

type Store struct {
	mu      sync.RWMutex
	dir     string
	batches map[string]*domain.RejuvenationBatch
	events  map[string][]audit.Event
	idem    map[string]any
	loadErr error
}

// New opens (or creates) the store rooted at dir. When dir is empty the store
// runs in-memory. If an existing snapshot cannot be read or deserialised the
// returned store is non-nil but poisoned: it retains the load error, serves
// reads from whatever partial state was decoded, and rejects all writes so the
// corrupt snapshot is preserved for offline inspection.
func New(dir string) *Store {
	s := &Store{dir: dir, batches: map[string]*domain.RejuvenationBatch{}, events: map[string][]audit.Event{}, idem: map[string]any{}}
	if dir != "" {
		os.MkdirAll(dir, 0755)
		s.loadErr = s.load()
	}
	return s
}

// LoadErr returns the error encountered while loading the on-disk snapshots, or
// nil when the store initialised cleanly. It lets callers distinguish a healthy
// store from one that refused to load its prior state.
func (s *Store) LoadErr() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadErr
}

func (s *Store) load() error {
	var firstErr error
	if err := loadJSON(filepath.Join(s.dir, "batches.json"), &s.batches); err != nil && !errors.Is(err, os.ErrNotExist) {
		firstErr = err
	}
	if err := loadJSON(filepath.Join(s.dir, "events.json"), &s.events); err != nil && !errors.Is(err, os.ErrNotExist) {
		if firstErr == nil {
			firstErr = err
		}
	}
	if err := loadJSON(filepath.Join(s.dir, "idempotency.json"), &s.idem); err != nil && !errors.Is(err, os.ErrNotExist) {
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("%w: %v", ErrSnapshotCorrupt, firstErr)
	}
	return nil
}

// loadJSON reads and unmarshals a snapshot file into target. Missing files are
// reported as os.ErrNotExist so callers can distinguish an absent (fresh)
// snapshot from a present but unreadable one; the latter includes truncation or
// any other illegal JSON.
func loadJSON(path string, target any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, target); err != nil {
		return err
	}
	return nil
}

func (s *Store) writable() error {
	if s.loadErr != nil {
		return fmt.Errorf("%w: %v", ErrSnapshotCorrupt, s.loadErr)
	}
	return nil
}

func (s *Store) persist() {
	if s.dir == "" {
		return
	}
	b, _ := json.Marshal(s.batches)
	os.WriteFile(filepath.Join(s.dir, "batches.json.tmp"), b, 0644)
	os.Rename(filepath.Join(s.dir, "batches.json.tmp"), filepath.Join(s.dir, "batches.json"))
	e, _ := json.Marshal(s.events)
	os.WriteFile(filepath.Join(s.dir, "events.json.tmp"), e, 0644)
	os.Rename(filepath.Join(s.dir, "events.json.tmp"), filepath.Join(s.dir, "events.json"))
	i, _ := json.Marshal(s.idem)
	os.WriteFile(filepath.Join(s.dir, "idempotency.json.tmp"), i, 0644)
	os.Rename(filepath.Join(s.dir, "idempotency.json.tmp"), filepath.Join(s.dir, "idempotency.json"))
}
func (s *Store) Get(id string) (*domain.RejuvenationBatch, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.batches[id]
	if !ok {
		return nil, false
	}
	return cloneBatch(b), true
}
func (s *Store) Save(b *domain.RejuvenationBatch, typ string, p any, expected int, req string) (*audit.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(b, typ, p, expected, req)
}

func (s *Store) saveLocked(b *domain.RejuvenationBatch, typ string, p any, expected int, req string) (*audit.Event, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	old := s.batches[b.BatchID]
	if old != nil {
		required := expected
		if required < 0 {
			required = b.Revision
		}
		if old.Revision != required {
			return nil, errors.New("revision_conflict")
		}
		if old.State == domain.StateArchived {
			return nil, errors.New("read_only")
		}
	}
	b.Revision++
	s.batches[b.BatchID] = cloneBatch(b)
	es := audit.Append(s.events[b.BatchID], typ, b.Revision, p)
	s.events[b.BatchID] = es
	if req != "" {
		s.idem[req] = cloneBatch(b)
	}
	s.persist()
	return &es[len(es)-1], nil
}

func (s *Store) SaveAndRemember(b *domain.RejuvenationBatch, typ string, p any, expected int, req string) (*audit.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req == "" {
		return s.saveLocked(b, typ, p, expected, "")
	}
	if _, ok := s.idem[req]; ok {
		return nil, errors.New("idempotency_replay")
	}
	return s.saveLocked(b, typ, p, expected, req)
}

func (s *Store) IdemBatch(req string) (*domain.RejuvenationBatch, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.idem[req]
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var b domain.RejuvenationBatch
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, false
	}
	return &b, true
}
func (s *Store) Events(id string) []audit.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]audit.Event(nil), s.events[id]...)
}
func (s *Store) Idem(req string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.idem[req]
	return v, ok
}
func (s *Store) PutIdem(req string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return
	}
	s.idem[req] = v
	s.persist()
}
