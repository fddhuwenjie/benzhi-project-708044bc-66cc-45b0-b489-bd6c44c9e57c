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
func (s *Store) persist() error {
	if s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	b, err := json.Marshal(s.batches)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, "batches.json.tmp"), b, 0644); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(s.dir, "batches.json.tmp"), filepath.Join(s.dir, "batches.json")); err != nil {
		return err
	}
	e, err := json.Marshal(s.events)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, "events.json.tmp"), e, 0644); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(s.dir, "events.json.tmp"), filepath.Join(s.dir, "events.json")); err != nil {
		return err
	}
	i, err := json.Marshal(s.idem)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, "idempotency.json.tmp"), i, 0644); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(s.dir, "idempotency.json.tmp"), filepath.Join(s.dir, "idempotency.json")); err != nil {
		return err
	}
	return nil
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
	// Stage all mutations before committing so that a persist failure leaves
	// the in-memory store at its previous committed state. audit.Append returns
	// a new slice, so keeping the old reference is enough to roll back.
	oldEvents := s.events[b.BatchID]
	var oldIdem any
	var hadIdem bool
	if req != "" {
		oldIdem, hadIdem = s.idem[req]
	}
	saved := cloneBatch(b)
	saved.Revision = b.Revision + 1
	events := audit.Append(oldEvents, typ, saved.Revision, p)
	s.batches[b.BatchID] = saved
	s.events[b.BatchID] = events
	if req != "" {
		s.idem[req] = cloneBatch(saved)
	}
	if err := s.persist(); err != nil {
		// Roll back to the pre-mutation state; nothing committed may remain.
		s.batches[b.BatchID] = old
		s.events[b.BatchID] = oldEvents
		if req != "" {
			if hadIdem {
				s.idem[req] = oldIdem
			} else {
				delete(s.idem, req)
			}
		}
		return nil, fmt.Errorf("persist_failed: %w", err)
	}
	b.Revision = saved.Revision
	return &events[len(events)-1], nil
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
func (s *Store) PutIdem(req string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idem[req] = v
	if err := s.persist(); err != nil {
		delete(s.idem, req)
		return err
	}
	return nil
}
