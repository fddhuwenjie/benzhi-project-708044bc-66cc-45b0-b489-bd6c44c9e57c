package persistence

import (
	"seedvault/internal/audit"
	"seedvault/internal/domain"
)

func (s *Store) List() []*domain.RejuvenationBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listCache != nil {
		return s.listCache
	}
	r := make([]*domain.RejuvenationBatch, 0, len(s.batches))
	for _, b := range s.batches {
		r = append(r, cloneBatch(b))
	}
	s.listCache = r
	return s.listCache
}
func (s *Store) Verify(id string) int { return audit.FirstBroken(s.Events(id)) }
