package persistence

import (
	"seedvault/internal/domain"
)

func cloneBatch(b *domain.RejuvenationBatch) *domain.RejuvenationBatch {
	if b == nil {
		return nil
	}
	c := *b
	return &c
}
