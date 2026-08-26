package persistence

import (
	"encoding/json"
	"seedvault/internal/domain"
)

func cloneBatch(b *domain.RejuvenationBatch) *domain.RejuvenationBatch {
	if b == nil {
		return nil
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil
	}
	var c domain.RejuvenationBatch
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil
	}
	return &c
}
