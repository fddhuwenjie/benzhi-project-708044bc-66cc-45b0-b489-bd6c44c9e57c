package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"seedvault/internal/audit"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"sort"
	"strings"
	"time"
)

type App struct{ Store *persistence.Store }

func New(s *persistence.Store) *App { return &App{Store: s} }
func newID() string                 { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func (a *App) Create(title, custodian string, items []domain.AccessionItem) (*domain.RejuvenationBatch, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(custodian) == "" || len(items) == 0 {
		return nil, errors.New("invalid_input")
	}
	for i := range items {
		items[i].AccessionID = strings.TrimSpace(items[i].AccessionID)
		items[i].TaxonName = strings.TrimSpace(items[i].TaxonName)
		items[i].CollectionSite = strings.TrimSpace(items[i].CollectionSite)
		items[i].SourceID = strings.TrimSpace(items[i].SourceID)
		if items[i].BaselineViability != 0 {
			items[i].BaselineViabilityProvided = true
		}
	}
	id := newID()
	now := time.Now().UTC()
	b := &domain.RejuvenationBatch{BatchID: id, Title: strings.TrimSpace(title), CustodianID: strings.TrimSpace(custodian), State: domain.StateDraft, CreatedAt: now, UpdatedAt: now, Items: items}
	for i := range b.Items {
		b.Items[i].BatchID = id
	}
	r := domain.DiagnoseDraft(b.Items)
	b.Readiness = &r
	if _, err := a.Store.Save(b, "batch_created", map[string]any{"title": b.Title, "custodian_id": b.CustodianID, "items": b.Items}, -1, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func (a *App) Get(id string) (*domain.RejuvenationBatch, error) {
	b, ok := a.Store.Get(id)
	if !ok {
		return nil, errors.New("not_found")
	}
	r := domain.DiagnoseDraft(b.Items)
	b.Readiness = &r
	return b, nil
}

// Identity keeps the former text-evidence application entry usable. New HTTP
// callers use IdentityStructured so all three evidence sources remain fielded.
func (a *App) Identity(id string, evidence map[string]string, confirmed []string, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	items := make([]domain.IdentityEvidenceItem, 0, len(evidence))
	for accession, ref := range evidence {
		item, ok := b.Accession(accession)
		if !ok {
			items = append(items, domain.IdentityEvidenceItem{AccessionID: accession})
			continue
		}
		doc := func(value any) domain.EvidenceDocument {
			return domain.EvidenceDocument{EvidenceID: strings.TrimSpace(ref), ClaimedValue: value, SourceRef: strings.TrimSpace(ref)}
		}
		items = append(items, domain.IdentityEvidenceItem{AccessionID: accession, CollectionRecord: doc(map[string]any{"collection_site": item.CollectionSite, "collection_year": item.CollectionYear}), TaxonomicIdentification: doc(item.TaxonName), StorageHistory: doc(item.StorageGeneration)})
	}
	return a.IdentityStructured(id, items, confirmed, expected)
}

func (a *App) IdentityStructured(id string, evidence []domain.IdentityEvidenceItem, confirmed []string, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateDraft {
		return nil, domain.ErrInvalidState
	}
	if b.Readiness == nil || !b.Readiness.Ready {
		return nil, domain.ValidationError{Code: "draft_not_ready", Details: b.Readiness}
	}
	for i := range evidence {
		evidence[i].AccessionID = strings.TrimSpace(evidence[i].AccessionID)
	}
	sort.SliceStable(evidence, func(i, j int) bool { return evidence[i].AccessionID < evidence[j].AccessionID })
	matrix := domain.BuildIdentityMatrix(b.Items, evidence)
	for _, entry := range matrix {
		if entry.Blocking {
			return nil, domain.ValidationError{Code: "identity_conflict", Details: map[string]any{"conflict_matrix": matrix}}
		}
	}
	seen := map[string]bool{}
	nonCustodian := false
	for i := range confirmed {
		confirmed[i] = strings.TrimSpace(confirmed[i])
		if confirmed[i] == "" || seen[confirmed[i]] {
			return nil, errors.New("identity_confirmers_invalid")
		}
		seen[confirmed[i]] = true
		if confirmed[i] != b.CustodianID {
			nonCustodian = true
		}
	}
	if len(confirmed) < 2 || !nonCustodian {
		return nil, errors.New("identity_confirmers_invalid")
	}
	sort.Strings(confirmed)
	identity := &domain.IdentityVerification{Evidence: evidence, Matrix: matrix, ConfirmedBy: confirmed}
	identity.Digest = audit.Digest(struct {
		Evidence []domain.IdentityEvidenceItem `json:"evidence"`
		Matrix   []domain.ConflictMatrixEntry  `json:"conflict_matrix"`
	}{evidence, matrix})
	b.Identity = identity
	b.IdentityConfirmedBy = append([]string(nil), confirmed...)
	if err := b.ValidateIdentity(); err != nil {
		return nil, err
	}
	if err := b.Transition(domain.StateIdentityVerified); err != nil {
		return nil, err
	}
	b.UpdatedAt = time.Now().UTC()
	payload := map[string]any{"evidence_digest": identity.Digest, "evidence_count": len(evidence), "confirmed_by": confirmed}
	if _, err := a.Store.Save(b, "identity_verified", payload, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func (a *App) Protocol(id string, protocol domain.GerminationProtocol, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateIdentityVerified {
		return nil, domain.ErrInvalidState
	}
	previous := b.Protocol
	if previous != nil && !canRelockProtocol(b) {
		return nil, errors.New("protocol_already_locked")
	}
	normalizeProtocol(b, &protocol)
	checks := domain.PreflightProtocol(b.Items, protocol)
	if !domain.ProtocolExecutable(checks) {
		return nil, domain.ValidationError{Code: "protocol_not_executable", Details: map[string]any{"preflight": checks}}
	}
	protocol.ProtocolID = newID()
	protocol.BatchID = id
	if previous != nil {
		if protocol.Version == 0 {
			protocol.Version = previous.Version + 1
		}
		if protocol.Version <= previous.Version {
			return nil, errors.New("protocol_version_invalid")
		}
		b.ProtocolHistory = append(b.ProtocolHistory, *previous)
		b.ObservationHistory = append(b.ObservationHistory, b.Observations...)
		b.RemediationHistory = append(b.RemediationHistory, b.Remediations...)
		b.Observations = nil
		b.Remediations = nil
	} else if protocol.Version == 0 {
		protocol.Version = 1
	}
	if protocol.Version < 1 {
		return nil, errors.New("protocol_version_invalid")
	}
	protocol.LockedAt = time.Now().UTC()
	protocol.Digest = audit.Digest(struct {
		Version int                        `json:"version"`
		Entries []domain.AccessionProtocol `json:"entries"`
	}{protocol.Version, protocol.Entries})
	b.Protocol = &protocol
	if err := b.Transition(domain.StateTrialActive); err != nil {
		return nil, err
	}
	b.UpdatedAt = time.Now().UTC()
	payload := map[string]any{"protocol_id": protocol.ProtocolID, "version": protocol.Version, "digest": protocol.Digest, "entries": protocol.Entries}
	if previous != nil {
		payload["supersedes_protocol_id"] = previous.ProtocolID
	}
	if _, err := a.Store.Save(b, "protocol_locked", payload, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func canRelockProtocol(b *domain.RejuvenationBatch) bool {
	if b.Review == nil || b.Review.Decision != "return" || b.Review.ReturnTarget != domain.StateIdentityVerified {
		return false
	}
	for _, check := range b.Review.Checks {
		if check.Conclusion == "fail" && (check.Category == "identity_chain" || check.Category == "protocol_deviation") {
			return true
		}
	}
	return false
}

func normalizeProtocol(b *domain.RejuvenationBatch, p *domain.GerminationProtocol) {
	if len(p.Entries) == 0 {
		groups := append([]domain.TreatmentAllocation(nil), p.GroupAllocations...)
		if len(groups) == 0 && len(p.TreatmentGroups) > 0 && p.SampleSize%len(p.TreatmentGroups) == 0 {
			for _, name := range p.TreatmentGroups {
				groups = append(groups, domain.TreatmentAllocation{Name: name, SampleSize: p.SampleSize / len(p.TreatmentGroups)})
			}
		}
		for _, item := range b.Items {
			p.Entries = append(p.Entries, domain.AccessionProtocol{AccessionID: item.AccessionID, SampleSize: p.SampleSize, TreatmentGroups: append([]domain.TreatmentAllocation(nil), groups...), TemperatureBounds: p.TemperatureBounds, HumidityBounds: p.HumidityBounds, ObservationDays: append([]int(nil), p.ObservationDays...), PassThreshold: p.PassThreshold})
		}
	}
	for i := range p.Entries {
		p.Entries[i].AccessionID = strings.TrimSpace(p.Entries[i].AccessionID)
		for j := range p.Entries[i].TreatmentGroups {
			p.Entries[i].TreatmentGroups[j].Name = strings.TrimSpace(p.Entries[i].TreatmentGroups[j].Name)
		}
		sort.SliceStable(p.Entries[i].TreatmentGroups, func(x, y int) bool {
			return p.Entries[i].TreatmentGroups[x].Name < p.Entries[i].TreatmentGroups[y].Name
		})
	}
	sort.SliceStable(p.Entries, func(i, j int) bool { return p.Entries[i].AccessionID < p.Entries[j].AccessionID })
	if len(p.Entries) > 0 {
		first := p.Entries[0]
		p.SampleSize, p.TemperatureBounds, p.HumidityBounds, p.ObservationDays, p.PassThreshold = first.SampleSize, first.TemperatureBounds, first.HumidityBounds, append([]int(nil), first.ObservationDays...), first.PassThreshold
		p.GroupAllocations = append([]domain.TreatmentAllocation(nil), first.TreatmentGroups...)
		p.TreatmentGroups = nil
		for _, group := range first.TreatmentGroups {
			p.TreatmentGroups = append(p.TreatmentGroups, group.Name)
		}
	}
}

func (a *App) Observe(id string, observation domain.TrialObservation, expected int) (*domain.RejuvenationBatch, error) {
	if _, err := a.ObserveBatch(id, []domain.TrialObservation{observation}, expected); err != nil {
		return nil, err
	}
	return a.Get(id)
}

func (a *App) ObserveBatch(id string, observations []domain.TrialObservation, expected int) (*domain.ObservationBatchResult, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateTrialActive {
		return nil, domain.ErrInvalidState
	}
	now := time.Now().UTC()
	for i := range observations {
		observations[i].ObservationID = newID()
		observations[i].BatchID = id
		observations[i].AccessionID = strings.TrimSpace(observations[i].AccessionID)
		observations[i].ObservedBy = strings.TrimSpace(observations[i].ObservedBy)
		observations[i].ObservedAt = now
		if observations[i].ObservedBy == "" {
			return nil, domain.ValidationError{Code: "observed_by_required", Details: map[string]any{"accession_id": observations[i].AccessionID}}
		}
	}
	if err := b.ValidateObservationBatch(observations); err != nil {
		if rule, ok := err.(domain.RuleError); ok {
			return nil, domain.ValidationError{Code: rule.Code, Details: map[string]any{"accession_id": rule.Detail}}
		}
		return nil, err
	}
	b.Observations = append(b.Observations, observations...)
	sort.SliceStable(b.Observations, func(i, j int) bool {
		if b.Observations[i].AccessionID != b.Observations[j].AccessionID {
			return b.Observations[i].AccessionID < b.Observations[j].AccessionID
		}
		return b.Observations[i].DayIndex < b.Observations[j].DayIndex
	})
	if b.LastObservationComplete() {
		_ = b.Transition(domain.StateTrialReview)
	}
	b.UpdatedAt = now
	payload := map[string]any{"day_index": observations[0].DayIndex, "observations": observations}
	if _, err := a.Store.Save(b, "observations_recorded", payload, expected, ""); err != nil {
		return nil, err
	}
	progress, completion, next := b.ObservationProgress()
	return &domain.ObservationBatchResult{BatchID: id, State: b.State, Revision: b.Revision, Observations: observations, Progress: progress, CompletionPercent: completion, NextObservationDay: next}, nil
}

func (a *App) CorrectObservation(id, observationID string, replacement domain.TrialObservation, reason, operator string, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateTrialActive {
		return nil, domain.ErrInvalidState
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(operator) == "" {
		return nil, errors.New("correction_metadata_required")
	}
	original, err := b.ReplaceObservation(observationID, replacement)
	if err != nil {
		return nil, err
	}
	if b.LastObservationComplete() {
		_ = b.Transition(domain.StateTrialReview)
	}
	b.UpdatedAt = time.Now().UTC()
	correction := domain.ObservationCorrection{ObservationID: observationID, Original: domain.CountsOf(original), Replacement: domain.CountsOf(replacement), Reason: strings.TrimSpace(reason), OperatorID: strings.TrimSpace(operator), CorrectedAt: b.UpdatedAt}
	payload := map[string]any{"observation_id": observationID, "original_digest": audit.Digest(correction.Original), "replacement_digest": audit.Digest(correction.Replacement), "reason": correction.Reason, "operator_id": correction.OperatorID, "original": correction.Original, "replacement": correction.Replacement, "corrected_at": correction.CorrectedAt}
	if _, err := a.Store.Save(b, "observation_corrected", payload, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func (a *App) Remediate(id string, remediation domain.RemediationCase, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateTrialReview && b.State != domain.StateRemediationActive {
		return nil, domain.ErrInvalidState
	}
	if _, ok := b.Accession(remediation.AccessionID); !ok || b.HasActiveCase(remediation.AccessionID) {
		return nil, errors.New("invalid_remediation")
	}
	if strings.TrimSpace(remediation.ReasonCode) == "" || len(remediation.EvidenceRefs) == 0 || strings.TrimSpace(remediation.TreatmentPlan) == "" || strings.TrimSpace(remediation.RetestProtocolID) == "" {
		return nil, errors.New("invalid_remediation")
	}
	remediation.CaseID, remediation.BatchID = newID(), id
	remediation.BeforeMetric = b.Metric(remediation.AccessionID)
	b.Remediations = append(b.Remediations, remediation)
	if b.State == domain.StateTrialReview {
		_ = b.Transition(domain.StateRemediationActive)
	}
	b.UpdatedAt = time.Now().UTC()
	if _, err := a.Store.Save(b, "remediation_started", remediation, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func (a *App) RetestCounts(id, caseID string, result domain.RetestResult, resolution, requestID string, expected int) (*domain.RejuvenationBatch, error) {
	return a.RetestCountsContext(context.Background(), id, caseID, result, resolution, requestID, expected)
}

func (a *App) RetestCountsContext(ctx context.Context, id, caseID string, result domain.RetestResult, resolution, requestID string, expected int) (*domain.RejuvenationBatch, error) {
	key := "retest:" + id + ":" + requestID
	if requestID != "" {
		if replay, ok := a.Store.IdemBatch(key); ok {
			return replay, nil
		}
	}
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateRemediationActive {
		return nil, domain.ErrInvalidState
	}
	index := -1
	for i := range b.Remediations {
		if b.Remediations[i].CaseID == caseID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, errors.New("unknown_case_id")
	}
	if b.Remediations[index].Resolution != "" || b.Remediations[index].Retest != nil {
		return nil, errors.New("case_already_closed")
	}
	if strings.TrimSpace(resolution) == "" {
		return nil, errors.New("resolution_required")
	}
	p, ok := b.ProtocolFor(b.Remediations[index].AccessionID)
	if !ok {
		return nil, errors.New("accession_protocol_missing")
	}
	derived, err := domain.BuildRetest(b.Remediations[index].BeforeMetric, p.PassThreshold, result.SampleSize, result.GerminatedCount, result.DormantCount, result.MoldedCount, result.DeadCount)
	if err != nil {
		return nil, err
	}
	b.Remediations[index].Retest = derived
	b.Remediations[index].AfterMetric = derived.AfterMetric
	b.Remediations[index].Resolution = strings.TrimSpace(resolution)
	if !b.HasOpenRemediation() {
		_ = b.Transition(domain.StateTrialReview)
	}
	b.UpdatedAt = time.Now().UTC()
	payload := map[string]any{"case_id": caseID, "retest": derived, "resolution": b.Remediations[index].Resolution, "request_id": requestID}
	remember := ""
	if requestID != "" {
		remember = key
	}
	if _, err := a.Store.SaveAndRememberContext(ctx, b, "remediation_completed", payload, expected, remember); err != nil {
		if err.Error() == "idempotency_replay" {
			if replay, ok := a.Store.IdemBatch(key); ok {
				return replay, nil
			}
		}
		return nil, err
	}
	return b, nil
}

// Retest is retained for application clients that used the original derived
// metric command. HTTP callers are required to submit raw counts.
func (a *App) Retest(id, caseID string, after float64, resolution string, expected int) (*domain.RejuvenationBatch, error) {
	if after < 0 || after > 100 || math.Mod(after, 1) != 0 {
		return nil, errors.New("invalid_retest")
	}
	return a.RetestCounts(id, caseID, domain.RetestResult{SampleSize: 100, GerminatedCount: int(after), DeadCount: 100 - int(after)}, resolution, "", expected)
}

func (a *App) ReviewChecklist(id, reviewer, summary, decision string, checks []domain.ReviewCheck, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateTrialReview {
		return nil, domain.ErrInvalidState
	}
	if strings.TrimSpace(reviewer) == "" {
		return nil, errors.New("reviewer_required")
	}
	for i := range checks {
		checks[i].Category = normalizeReviewCategory(checks[i].Category)
		if checks[i].Conclusion == "passed" {
			checks[i].Conclusion = "pass"
		}
		if checks[i].Conclusion == "failed" {
			checks[i].Conclusion = "fail"
		}
	}
	if err := domain.ValidateReviewChecks(checks); err != nil {
		return nil, err
	}
	events := a.Store.Events(id)
	for _, check := range checks {
		if !reviewEvidenceKnown(b, events, check) {
			return nil, domain.ValidationError{Code: "review_evidence_reference_invalid", Details: map[string]any{"category": check.Category}}
		}
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Category < checks[j].Category })
	facts := reviewFacts(b)
	for _, check := range checks {
		if check.Conclusion == "pass" && !facts[check.Category] {
			return nil, domain.ValidationError{Code: "review_check_conclusion_mismatch", Details: map[string]any{"category": check.Category}}
		}
	}
	record := &domain.ReviewRecord{Checks: checks, Decision: decision, ReviewerID: strings.TrimSpace(reviewer), Summary: strings.TrimSpace(summary)}
	if decision == "return" {
		target, failed, err := domain.ReviewReturnTarget(checks)
		if err != nil {
			return nil, err
		}
		record.ReturnTarget = target
		b.Review, b.ReviewSummary = record, record.Summary
		if err := b.ReturnFromReview(target); err != nil {
			return nil, err
		}
		b.UpdatedAt = time.Now().UTC()
		payload := map[string]any{"failed_checks": failed, "return_target": target, "reviewer_id": record.ReviewerID, "summary": record.Summary}
		if _, err := a.Store.Save(b, "review_returned", payload, expected, ""); err != nil {
			return nil, err
		}
		return b, nil
	}
	if decision != "approve" {
		return nil, errors.New("review_decision_invalid")
	}
	if record.ReviewerID == b.CustodianID || record.Summary == "" {
		return nil, errors.New("review_invalid")
	}
	for _, check := range checks {
		if check.Conclusion != "pass" {
			return nil, errors.New("review_checklist_not_all_passed")
		}
	}
	if b.HasOpenRemediation() {
		return nil, errors.New("remediation_incomplete")
	}
	b.ReviewerID, b.ReviewSummary, b.Review = record.ReviewerID, record.Summary, record
	if err := b.Transition(domain.StateApproved); err != nil {
		return nil, err
	}
	b.UpdatedAt = time.Now().UTC()
	payload := map[string]any{"reviewer_id": record.ReviewerID, "summary": record.Summary, "checklist_digest": audit.Digest(checks), "checks": checks}
	if _, err := a.Store.Save(b, "review_approved", payload, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

func reviewEvidenceKnown(b *domain.RejuvenationBatch, events []audit.Event, check domain.ReviewCheck) bool {
	known := map[string]bool{"aggregate:" + check.Category: true, b.BatchID: true}
	eventTypes := map[string]map[string]bool{
		"identity_chain":           {"batch_created": true, "identity_verified": true},
		"protocol_deviation":       {"protocol_locked": true},
		"observation_completeness": {"observations_recorded": true, "observation_corrected": true},
		"threshold_determination":  {"observations_recorded": true, "observation_corrected": true, "remediation_completed": true},
		"remediation_closure":      {"remediation_started": true, "remediation_completed": true},
	}
	for _, event := range events {
		if eventTypes[check.Category][event.Type] {
			known[event.Hash], known[event.Type], known[fmt.Sprintf("revision:%d", event.Revision)] = true, true, true
		}
	}
	switch check.Category {
	case "identity_chain":
		if b.Identity != nil {
			known[b.Identity.Digest] = true
			for _, item := range b.Identity.Evidence {
				known[item.CollectionRecord.EvidenceID], known[item.TaxonomicIdentification.EvidenceID], known[item.StorageHistory.EvidenceID] = true, true, true
			}
		}
	case "protocol_deviation":
		if b.Protocol != nil {
			known[b.Protocol.ProtocolID], known[b.Protocol.Digest] = true, true
		}
	case "observation_completeness", "threshold_determination":
		for _, observation := range b.Observations {
			known[observation.ObservationID] = true
		}
		for _, remediation := range b.Remediations {
			known[remediation.CaseID] = true
		}
	case "remediation_closure":
		for _, remediation := range b.Remediations {
			known[remediation.CaseID], known[remediation.RetestProtocolID] = true, true
		}
	}
	for _, ref := range check.EvidenceRefs {
		if known[strings.TrimSpace(ref)] {
			return true
		}
	}
	return false
}

func normalizeReviewCategory(category string) string {
	switch strings.TrimSpace(category) {
	case "identity", "identity_evidence", "identity_chain":
		return "identity_chain"
	case "protocol", "protocol_deviations", "protocol_deviation":
		return "protocol_deviation"
	case "observations", "observation_completeness":
		return "observation_completeness"
	case "threshold", "threshold_assessment", "threshold_determination":
		return "threshold_determination"
	case "retest_closure", "remediation", "remediation_closure":
		return "remediation_closure"
	default:
		return strings.TrimSpace(category)
	}
}

func reviewFacts(b *domain.RejuvenationBatch) map[string]bool {
	identityOK := b.Identity != nil
	if identityOK {
		for _, x := range b.Identity.Matrix {
			if x.Blocking {
				identityOK = false
				break
			}
		}
	}
	thresholdOK := b.Protocol != nil
	if thresholdOK {
		for _, item := range b.Items {
			p, ok := b.ProtocolFor(item.AccessionID)
			if !ok || b.Metric(item.AccessionID) < p.PassThreshold {
				thresholdOK = false
				break
			}
		}
	}
	return map[string]bool{"identity_chain": identityOK, "protocol_deviation": b.Protocol != nil, "observation_completeness": b.LastObservationComplete(), "threshold_determination": thresholdOK, "remediation_closure": !b.HasOpenRemediation()}
}

func (a *App) Review(id, reviewer, summary string, approve bool, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	facts := reviewFacts(b)
	checks := make([]domain.ReviewCheck, 0, len(domain.ReviewCategories))
	for _, category := range domain.ReviewCategories {
		conclusion := "fail"
		if facts[category] {
			conclusion = "pass"
		}
		checks = append(checks, domain.ReviewCheck{Category: category, Conclusion: conclusion, EvidenceRefs: []string{"aggregate:" + category}, Comment: summary})
	}
	decision := "return"
	if approve {
		decision = "approve"
	}
	return a.ReviewChecklist(id, reviewer, summary, decision, checks, expected)
}

func (a *App) Archive(id, grade, due string, expected int) (*domain.RejuvenationBatch, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != domain.StateApproved {
		return nil, domain.ErrInvalidState
	}
	if grade != "A" && grade != "B" && grade != "C" {
		return nil, errors.New("invalid_release_grade")
	}
	if _, err := time.Parse("2006-01-02", due); err != nil {
		return nil, errors.New("invalid_recheck_due_date")
	}
	events := a.Store.Events(id)
	if err := audit.Verify(events); err != nil {
		return nil, err
	}
	chainHead := ""
	if len(events) > 0 {
		chainHead = events[len(events)-1].Hash
	}
	decision := audit.ReleaseDecision{ReleaseGrade: grade, RecheckDueDate: due, ApprovedBy: b.ReviewerID, Review: b.Review}
	contents := audit.SegmentContents(b, decision)
	segments, err := audit.SegmentManifest(contents)
	if err != nil {
		return nil, err
	}
	pkg := &domain.ReleaseEvidencePackage{PackageID: newID(), BatchID: id, ReleaseGrade: grade, RecheckDueDate: due, ManifestDigest: audit.ManifestDigest(segments, chainHead), EventChainHead: chainHead, ApprovedBy: b.ReviewerID, ArchivedAt: time.Now().UTC(), Segments: segments}
	b.Release = pkg
	if err := b.Transition(domain.StateArchived); err != nil {
		return nil, err
	}
	b.UpdatedAt = pkg.ArchivedAt
	if _, err := a.Store.Save(b, "archived", pkg, expected, ""); err != nil {
		return nil, err
	}
	return b, nil
}

type SegmentStatus struct {
	Name           string `json:"name"`
	ExpectedDigest string `json:"expected_digest"`
	ActualDigest   string `json:"actual_digest"`
	Valid          bool   `json:"valid"`
}

type IntegrityResult struct {
	Valid                   bool            `json:"valid"`
	Segments                []SegmentStatus `json:"segments"`
	ManifestDigestValid     bool            `json:"manifest_digest_valid"`
	EventRevisionContinuous bool            `json:"event_revision_continuous"`
	EventChainValid         bool            `json:"event_chain_valid"`
	ChainHeadValid          bool            `json:"chain_head_valid"`
	FirstBrokenSegment      string          `json:"first_broken_segment,omitempty"`
	FirstBrokenRevision     int             `json:"first_broken_revision,omitempty"`
}

func (a *App) DiagnoseIntegrity(id string) (*IntegrityResult, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.Release == nil {
		return nil, errors.New("evidence_package_not_found")
	}
	events := a.Store.Events(id)
	contents := audit.SegmentContents(b, audit.DecisionFromPackage(b))
	actual, _ := audit.SegmentManifest(contents)
	expected := map[string]string{}
	for _, x := range b.Release.Segments {
		expected[x.Name] = x.Digest
	}
	result := &IntegrityResult{Valid: true, EventRevisionContinuous: true, EventChainValid: true, ChainHeadValid: true}
	for _, x := range actual {
		status := SegmentStatus{Name: x.Name, ExpectedDigest: expected[x.Name], ActualDigest: x.Digest, Valid: expected[x.Name] == x.Digest}
		if !status.Valid && result.FirstBrokenSegment == "" {
			result.FirstBrokenSegment = x.Name
			result.FirstBrokenRevision = firstRevisionForSegment(events, x.Name)
			result.Valid = false
		}
		result.Segments = append(result.Segments, status)
	}
	result.ManifestDigestValid = audit.ManifestDigest(actual, b.Release.EventChainHead) == b.Release.ManifestDigest
	if !result.ManifestDigestValid {
		result.Valid = false
		if result.FirstBrokenSegment == "" {
			result.FirstBrokenSegment = "manifest"
		}
	}
	prev := ""
	for i, event := range events {
		if event.Revision != i+1 {
			result.EventRevisionContinuous = false
			if result.FirstBrokenRevision == 0 {
				result.FirstBrokenRevision = i + 1
			}
		}
		if audit.HashEvent(prev, event) != event.Hash {
			result.EventChainValid = false
			if result.FirstBrokenRevision == 0 {
				result.FirstBrokenRevision = event.Revision
			}
		}
		prev = event.Hash
	}
	archivePrev := ""
	for _, event := range events {
		if event.Type == "archived" {
			archivePrev = event.PrevHash
			break
		}
	}
	result.ChainHeadValid = archivePrev != "" && archivePrev == b.Release.EventChainHead
	if !result.EventRevisionContinuous || !result.EventChainValid || !result.ChainHeadValid {
		result.Valid = false
		if result.FirstBrokenSegment == "" {
			result.FirstBrokenSegment = "event_chain"
		}
	}
	return result, nil
}

func firstRevisionForSegment(events []audit.Event, segment string) int {
	types := map[string]map[string]bool{
		"identity":         {"batch_created": true, "identity_verified": true},
		"protocol":         {"protocol_locked": true},
		"observation":      {"observations_recorded": true, "observation_recorded": true, "observation_corrected": true},
		"remediation":      {"remediation_started": true, "remediation_completed": true},
		"release_decision": {"review_approved": true, "archived": true},
	}
	for _, event := range events {
		if types[segment][event.Type] {
			return event.Revision
		}
	}
	return 0
}

func (a *App) Integrity(id string) error {
	result, err := a.DiagnoseIntegrity(id)
	if err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("integrity_invalid")
	}
	return nil
}

func (a *App) EvidencePackage(id, segment string) (map[string]any, error) {
	b, err := a.Get(id)
	if err != nil {
		return nil, err
	}
	if b.Release == nil {
		return nil, errors.New("not_found")
	}
	contents := audit.SegmentContents(b, audit.DecisionFromPackage(b))
	if segment == "observations" {
		segment = "observation"
	}
	out := map[string]any{"package_id": b.Release.PackageID, "manifest_digest": b.Release.ManifestDigest, "event_chain_head": b.Release.EventChainHead, "segments": b.Release.Segments}
	if segment != "" {
		value, ok := contents[segment]
		if !ok {
			return nil, errors.New("unknown_evidence_segment")
		}
		out["segment"] = segment
		out["content"] = value
		for _, digest := range b.Release.Segments {
			if digest.Name == segment {
				out["segment_digest"] = digest.Digest
			}
		}
	} else {
		out["contents"] = contents
	}
	return out, nil
}
