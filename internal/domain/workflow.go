package domain

import (
	"errors"
	"sort"
	"strings"
)

var ReviewCategories = []string{"identity_chain", "protocol_deviation", "observation_completeness", "threshold_determination", "remediation_closure"}

func BuildRetest(before, threshold float64, sampleSize, germinated, dormant, molded, dead int) (*RetestResult, error) {
	if sampleSize <= 0 || germinated < 0 || dormant < 0 || molded < 0 || dead < 0 || germinated+dormant+molded+dead != sampleSize {
		return nil, errors.New("retest_count_mismatch")
	}
	after := Viability(germinated, sampleSize)
	effect := "no_improvement"
	if after >= threshold {
		effect = "threshold_met"
	} else if after-before >= 10 {
		effect = "significantly_improved"
	}
	return &RetestResult{SampleSize: sampleSize, GerminatedCount: germinated, DormantCount: dormant, MoldedCount: molded, DeadCount: dead, BeforeMetric: before, AfterMetric: after, Improvement: after - before, Effect: effect}, nil
}

func ValidateReviewChecks(checks []ReviewCheck) error {
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, category := range ReviewCategories {
		allowed[category] = true
	}
	for _, check := range checks {
		if !allowed[check.Category] || seen[check.Category] {
			return errors.New("review_checklist_invalid")
		}
		if check.Conclusion != "pass" && check.Conclusion != "fail" {
			return errors.New("review_checklist_invalid")
		}
		if len(check.EvidenceRefs) == 0 || strings.TrimSpace(check.Comment) == "" {
			return errors.New("review_checklist_invalid")
		}
		seen[check.Category] = true
	}
	if len(seen) != len(ReviewCategories) {
		return errors.New("review_checklist_incomplete")
	}
	return nil
}

func ReviewReturnTarget(checks []ReviewCheck) (State, []ReviewCheck, error) {
	failed := make([]ReviewCheck, 0)
	for _, check := range checks {
		if check.Conclusion == "fail" {
			failed = append(failed, check)
		}
	}
	if len(failed) == 0 {
		return "", nil, errors.New("failed_review_check_required")
	}
	sort.SliceStable(failed, func(i, j int) bool { return failed[i].Category < failed[j].Category })
	target := StateRemediationActive
	for _, check := range failed {
		if strings.TrimSpace(check.Comment) == "" {
			return "", nil, errors.New("review_return_reason_required")
		}
		switch check.Category {
		case "identity_chain", "protocol_deviation":
			target = StateIdentityVerified
		case "observation_completeness", "threshold_determination":
			if target != StateIdentityVerified {
				target = StateTrialActive
			}
		case "remediation_closure":
			// remediation_active is the least invasive target.
		}
	}
	return target, failed, nil
}

func (b *RejuvenationBatch) ReturnFromReview(target State) error {
	if b.State != StateTrialReview {
		return ErrInvalidState
	}
	if target != StateIdentityVerified && target != StateTrialActive && target != StateRemediationActive {
		return ErrInvalidState
	}
	b.State = target
	return nil
}

func (b *RejuvenationBatch) HasOpenRemediation() bool {
	for _, c := range b.Remediations {
		if c.Resolution == "" || c.Retest == nil {
			return true
		}
	}
	return false
}
