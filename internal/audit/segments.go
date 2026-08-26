package audit

import (
	"errors"
	"seedvault/internal/domain"
)

var SegmentOrder = []string{"identity", "protocol", "observation", "remediation", "release_decision"}

type ReleaseDecision struct {
	ReleaseGrade   string               `json:"release_grade"`
	RecheckDueDate string               `json:"recheck_due_date"`
	ApprovedBy     string               `json:"approved_by"`
	Review         *domain.ReviewRecord `json:"review"`
}

func SegmentContents(b *domain.RejuvenationBatch, decision ReleaseDecision) map[string]any {
	return map[string]any{
		"identity":         map[string]any{"items": b.Items, "identity": b.Identity},
		"protocol":         map[string]any{"current": b.Protocol, "history": b.ProtocolHistory},
		"observation":      map[string]any{"current": b.Observations, "history": b.ObservationHistory},
		"remediation":      map[string]any{"current": b.Remediations, "history": b.RemediationHistory},
		"release_decision": decision,
	}
}

func SegmentManifest(contents map[string]any) ([]domain.SegmentDigest, error) {
	segments := make([]domain.SegmentDigest, 0, len(SegmentOrder))
	for _, name := range SegmentOrder {
		value, ok := contents[name]
		if !ok {
			return nil, errors.New("manifest_segment_missing")
		}
		segments = append(segments, domain.SegmentDigest{Name: name, Digest: Digest(value)})
	}
	return segments, nil
}

func ManifestDigest(segments []domain.SegmentDigest, eventChainHead string) string {
	return Digest(struct {
		Segments       []domain.SegmentDigest `json:"segments"`
		EventChainHead string                 `json:"event_chain_head"`
	}{segments, eventChainHead})
}

func DecisionFromPackage(b *domain.RejuvenationBatch) ReleaseDecision {
	if b.Release == nil {
		return ReleaseDecision{}
	}
	return ReleaseDecision{ReleaseGrade: b.Release.ReleaseGrade, RecheckDueDate: b.Release.RecheckDueDate, ApprovedBy: b.Release.ApprovedBy, Review: b.Review}
}
