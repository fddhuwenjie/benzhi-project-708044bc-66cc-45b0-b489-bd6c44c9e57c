package persistence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"seedvault/internal/audit"
	"seedvault/internal/domain"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	dir     string
	batches map[string]*domain.RejuvenationBatch
	events  map[string][]audit.Event
	idem    map[string]any
}

func New(dir string) *Store {
	s := &Store{dir: dir, batches: map[string]*domain.RejuvenationBatch{}, events: map[string][]audit.Event{}, idem: map[string]any{}}
	if dir != "" {
		os.MkdirAll(dir, 0755)
		s.load()
	}
	return s
}
func (s *Store) load() {
	b, _ := os.ReadFile(filepath.Join(s.dir, "batches.json"))
	json.Unmarshal(b, &s.batches)
	e, _ := os.ReadFile(filepath.Join(s.dir, "events.json"))
	json.Unmarshal(e, &s.events)
	i, _ := os.ReadFile(filepath.Join(s.dir, "idempotency.json"))
	json.Unmarshal(i, &s.idem)
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
	s.idem[req] = v
	s.persist()
}
