package domain

import "testing"

func TestTransition(t *testing.T) {
	b := &RejuvenationBatch{State: StateDraft}
	if b.Transition(StateIdentityVerified) != nil {
		t.Fatal()
	}
	if b.Transition(StateArchived) == nil {
		t.Fatal()
	}
}

func TestReviewReturnTargetsObservationStage(t *testing.T) {
	checks := make([]ReviewCheck, 0, len(ReviewCategories))
	for _, category := range ReviewCategories {
		conclusion := "pass"
		if category == "observation_completeness" {
			conclusion = "fail"
		}
		checks = append(checks, ReviewCheck{Category: category, Conclusion: conclusion, EvidenceRefs: []string{"event"}, Comment: "复核意见"})
	}
	if err := ValidateReviewChecks(checks); err != nil {
		t.Fatal(err)
	}
	target, failed, err := ReviewReturnTarget(checks)
	if err != nil || target != StateTrialActive || len(failed) != 1 {
		t.Fatalf("target=%s failed=%d err=%v", target, len(failed), err)
	}
	protocol := &GerminationProtocol{ProtocolID: "locked"}
	b := &RejuvenationBatch{State: StateTrialReview, Protocol: protocol, Identity: &IdentityVerification{Digest: "identity"}}
	if err := b.ReturnFromReview(target); err != nil {
		t.Fatal(err)
	}
	if b.Protocol != protocol || b.Identity == nil {
		t.Fatal("review return changed locked evidence")
	}
}
