package persistence

import (
	"seedvault/internal/audit"
	"seedvault/internal/domain"
)

func (s *Store) List() []*domain.RejuvenationBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := make([]*domain.RejuvenationBatch, 0, len(s.batches))
	for _, b := range s.batches {
		r = append(r, cloneBatch(b))
	}
	return r
}
func (s *Store) Verify(id string) int { return audit.FirstBroken(s.Events(id)) }
