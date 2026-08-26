package domain

import (
	"errors"
	"fmt"
	"time"
)

type State string

const (
	StateDraft             State = "draft"
	StateIdentityVerified  State = "identity_verified"
	StateTrialActive       State = "trial_active"
	StateTrialReview       State = "trial_review"
	StateRemediationActive State = "remediation_active"
	StateApproved          State = "approved"
	StateArchived          State = "archived"
)

type AccessionItem struct {
	AccessionID               string  `json:"accession_id"`
	BatchID                   string  `json:"batch_id"`
	TaxonName                 string  `json:"taxon_name"`
	CollectionSite            string  `json:"collection_site"`
	CollectionYear            int     `json:"collection_year"`
	StorageGeneration         int     `json:"storage_generation"`
	SeedCount                 int     `json:"seed_count"`
	BaselineViability         float64 `json:"baseline_viability"`
	SourceID                  string  `json:"source_id"`
	BaselineViabilityProvided bool    `json:"-"`
}

type Diagnostic struct {
	AccessionID string `json:"accession_id,omitempty"`
	Field       string `json:"field"`
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
}

type DraftReadiness struct {
	Ready             bool         `json:"ready"`
	ItemCount         int          `json:"item_count"`
	TotalSeedCount    int          `json:"total_seed_count"`
	LowViabilityCount int          `json:"low_viability_count"`
	MissingFieldCount int          `json:"missing_field_count"`
	BlockingCount     int          `json:"blocking_count"`
	Diagnostics       []Diagnostic `json:"diagnostics"`
}

type EvidenceDocument struct {
	EvidenceID     string         `json:"evidence_id"`
	ClaimedValue   any            `json:"claimed_value"`
	SourceRef      string         `json:"source_ref"`
	CollectionSite string         `json:"collection_site,omitempty"`
	CollectionYear int            `json:"collection_year,omitempty"`
	TaxonName      string         `json:"taxon_name,omitempty"`
	Generation     int            `json:"storage_generation,omitempty"`
	Claims         map[string]any `json:"claims,omitempty"`
}

type IdentityEvidenceItem struct {
	AccessionID             string           `json:"accession_id"`
	CollectionRecord        EvidenceDocument `json:"collection_record"`
	TaxonomicIdentification EvidenceDocument `json:"taxonomic_identification"`
	StorageHistory          EvidenceDocument `json:"storage_history"`
}

type ConflictMatrixEntry struct {
	AccessionID string `json:"accession_id"`
	Evidence    string `json:"evidence"`
	Field       string `json:"field"`
	Result      string `json:"result"`
	Expected    any    `json:"expected,omitempty"`
	Claimed     any    `json:"claimed,omitempty"`
	Blocking    bool   `json:"blocking"`
	Message     string `json:"message,omitempty"`
}

type IdentityVerification struct {
	Evidence    []IdentityEvidenceItem `json:"evidence"`
	Matrix      []ConflictMatrixEntry  `json:"conflict_matrix"`
	ConfirmedBy []string               `json:"confirmed_by"`
	Digest      string                 `json:"digest"`
}

type GerminationProtocol struct {
	ProtocolID        string                `json:"protocol_id"`
	BatchID           string                `json:"batch_id"`
	Version           int                   `json:"version"`
	SampleSize        int                   `json:"sample_size"`
	TreatmentGroups   []string              `json:"treatment_groups"`
	GroupAllocations  []TreatmentAllocation `json:"group_allocations,omitempty"`
	TemperatureBounds [2]float64            `json:"temperature_bounds"`
	HumidityBounds    [2]float64            `json:"humidity_bounds"`
	ObservationDays   []int                 `json:"observation_days"`
	PassThreshold     float64               `json:"pass_threshold"`
	LockedAt          time.Time             `json:"locked_at"`
	Entries           []AccessionProtocol   `json:"entries"`
	Digest            string                `json:"digest"`
}
type TreatmentAllocation struct {
	Name       string `json:"name"`
	SampleSize int    `json:"sample_size"`
}
type AccessionProtocol struct {
	AccessionID       string                `json:"accession_id"`
	SampleSize        int                   `json:"sample_size"`
	TreatmentGroups   []TreatmentAllocation `json:"treatment_groups"`
	TemperatureBounds [2]float64            `json:"temperature_bounds"`
	HumidityBounds    [2]float64            `json:"humidity_bounds"`
	ObservationDays   []int                 `json:"observation_days"`
	PassThreshold     float64               `json:"pass_threshold"`
}
type ProtocolCheck struct {
	AccessionID string       `json:"accession_id"`
	Executable  bool         `json:"executable"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}
type TrialObservation struct {
	ObservationID   string    `json:"observation_id"`
	BatchID         string    `json:"batch_id"`
	AccessionID     string    `json:"accession_id"`
	ObservedBy      string    `json:"observed_by"`
	DayIndex        int       `json:"day_index"`
	GerminatedCount int       `json:"germinated_count"`
	DormantCount    int       `json:"dormant_count"`
	MoldedCount     int       `json:"molded_count"`
	DeadCount       int       `json:"dead_count"`
	ObservedAt      time.Time `json:"observed_at"`
	CorrectionCount int       `json:"correction_count"`
}
type ObservationCorrection struct {
	ObservationID string    `json:"observation_id"`
	Original      Counts    `json:"original"`
	Replacement   Counts    `json:"replacement"`
	Reason        string    `json:"reason"`
	OperatorID    string    `json:"operator_id"`
	CorrectedAt   time.Time `json:"corrected_at"`
}
type Counts struct {
	GerminatedCount int `json:"germinated_count"`
	DormantCount    int `json:"dormant_count"`
	MoldedCount     int `json:"molded_count"`
	DeadCount       int `json:"dead_count"`
}
type ObservationProgress struct {
	AccessionID         string  `json:"accession_id"`
	CumulativeCount     int     `json:"cumulative_count"`
	CumulativeViability float64 `json:"cumulative_viability"`
	CompletionPercent   float64 `json:"completion_percent"`
	NextObservationDay  *int    `json:"next_observation_day"`
}
type ObservationBatchResult struct {
	BatchID            string                `json:"batch_id"`
	State              State                 `json:"state"`
	Revision           int                   `json:"revision"`
	Observations       []TrialObservation    `json:"observations"`
	Progress           []ObservationProgress `json:"progress"`
	CompletionPercent  float64               `json:"completion_percent"`
	NextObservationDay *int                  `json:"next_observation_day"`
}
type RemediationCase struct {
	CaseID           string        `json:"case_id"`
	BatchID          string        `json:"batch_id"`
	AccessionID      string        `json:"accession_id"`
	ReasonCode       string        `json:"reason_code"`
	EvidenceRefs     []string      `json:"evidence_refs"`
	TreatmentPlan    string        `json:"treatment_plan"`
	RetestProtocolID string        `json:"retest_protocol_id"`
	BeforeMetric     float64       `json:"before_metric"`
	AfterMetric      float64       `json:"after_metric"`
	Resolution       string        `json:"resolution"`
	Retest           *RetestResult `json:"retest,omitempty"`
}
type RetestResult struct {
	SampleSize      int     `json:"sample_size"`
	GerminatedCount int     `json:"germinated_count"`
	DormantCount    int     `json:"dormant_count"`
	MoldedCount     int     `json:"molded_count"`
	DeadCount       int     `json:"dead_count"`
	BeforeMetric    float64 `json:"before_metric"`
	AfterMetric     float64 `json:"after_metric"`
	Improvement     float64 `json:"improvement"`
	Effect          string  `json:"effect"`
}
type ReviewCheck struct {
	Category     string   `json:"category"`
	Conclusion   string   `json:"conclusion"`
	EvidenceRefs []string `json:"evidence_refs"`
	Comment      string   `json:"comment"`
}
type ReviewRecord struct {
	Checks       []ReviewCheck `json:"checks"`
	Decision     string        `json:"decision"`
	ReviewerID   string        `json:"reviewer_id"`
	Summary      string        `json:"summary"`
	ReturnTarget State         `json:"return_target,omitempty"`
}
type SegmentDigest struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}
type ReleaseEvidencePackage struct {
	PackageID      string          `json:"package_id"`
	BatchID        string          `json:"batch_id"`
	ReleaseGrade   string          `json:"release_grade"`
	RecheckDueDate string          `json:"recheck_due_date"`
	ManifestDigest string          `json:"manifest_digest"`
	EventChainHead string          `json:"event_chain_head"`
	ApprovedBy     string          `json:"approved_by"`
	ArchivedAt     time.Time       `json:"archived_at"`
	Segments       []SegmentDigest `json:"segments"`
}
type RejuvenationBatch struct {
	BatchID             string                  `json:"batch_id"`
	Title               string                  `json:"title"`
	CustodianID         string                  `json:"custodian_id"`
	ReviewerID          string                  `json:"reviewer_id"`
	State               State                   `json:"state"`
	Revision            int                     `json:"revision"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
	Items               []AccessionItem         `json:"items"`
	Protocol            *GerminationProtocol    `json:"protocol,omitempty"`
	ProtocolHistory     []GerminationProtocol   `json:"protocol_history,omitempty"`
	Observations        []TrialObservation      `json:"observations"`
	ObservationHistory  []TrialObservation      `json:"observation_history,omitempty"`
	Remediations        []RemediationCase       `json:"remediations"`
	RemediationHistory  []RemediationCase       `json:"remediation_history,omitempty"`
	Release             *ReleaseEvidencePackage `json:"release,omitempty"`
	Readiness           *DraftReadiness         `json:"readiness,omitempty"`
	Identity            *IdentityVerification   `json:"identity,omitempty"`
	IdentityConfirmedBy []string                `json:"identity_confirmed_by"`
	ReviewSummary       string                  `json:"review_summary"`
	Review              *ReviewRecord           `json:"review,omitempty"`
}

var ErrInvalidState = errors.New("invalid_state")

func (b *RejuvenationBatch) Transition(to State) error {
	valid := map[State][]State{StateDraft: {StateIdentityVerified}, StateIdentityVerified: {StateTrialActive}, StateTrialActive: {StateTrialReview}, StateTrialReview: {StateRemediationActive, StateApproved}, StateRemediationActive: {StateTrialReview}, StateApproved: {StateArchived}}
	for _, s := range valid[b.State] {
		if s == to {
			b.State = to
			return nil
		}
	}
	return fmt.Errorf("%w: %s to %s", ErrInvalidState, b.State, to)
}
func (b *RejuvenationBatch) ValidateIdentity() error {
	if len(b.Items) == 0 || b.Identity == nil || len(b.Identity.Evidence) < len(b.Items) || len(b.IdentityConfirmedBy) < 2 {
		return errors.New("identity_incomplete")
	}
	seen := map[string]bool{}
	for _, id := range b.IdentityConfirmedBy {
		if id == "" || seen[id] {
			return errors.New("identity_confirmers_invalid")
		}
		seen[id] = true
	}
	if b.CustodianID != "" && seen[b.CustodianID] && len(seen) == 1 {
		return errors.New("identity_confirmers_invalid")
	}
	for _, x := range b.Identity.Matrix {
		if x.Blocking {
			return errors.New("identity_conflict")
		}
	}
	return nil
}
func (b *RejuvenationBatch) ValidateObservation(o TrialObservation) error {
	if _, ok := b.Accession(o.AccessionID); !ok {
		return errors.New("unknown_accession")
	}
	if o.DayIndex <= 0 {
		return errors.New("invalid_day_index")
	}
	if o.GerminatedCount < 0 || o.DormantCount < 0 || o.MoldedCount < 0 || o.DeadCount < 0 {
		return errors.New("count_negative")
	}
	if o.GerminatedCount+o.DormantCount+o.MoldedCount+o.DeadCount <= 0 {
		return errors.New("count_empty")
	}
	p, ok := b.ProtocolFor(o.AccessionID)
	if !ok {
		return errors.New("accession_protocol_missing")
	}
	if o.GerminatedCount+o.DormantCount+o.MoldedCount+o.DeadCount > p.SampleSize {
		return errors.New("count_exceeds_seed")
	}
	total := o.GerminatedCount + o.DormantCount + o.MoldedCount + o.DeadCount
	for _, x := range b.Observations {
		if x.AccessionID == o.AccessionID {
			total += x.GerminatedCount + x.DormantCount + x.MoldedCount + x.DeadCount
		}
	}
	if total > p.SampleSize {
		return errors.New("cumulative_count_exceeds_seed")
	}
	for _, x := range b.Observations {
		if x.AccessionID == o.AccessionID && o.DayIndex <= x.DayIndex {
			return errors.New("observation_order")
		}
	}
	return nil
}

func (b *RejuvenationBatch) HasActiveCase(accession string) bool {
	for _, r := range b.Remediations {
		if r.AccessionID == accession && r.Resolution == "" {
			return true
		}
	}
	return false
}
func (b *RejuvenationBatch) Metric(id string) float64 {
	for _, r := range b.Remediations {
		if r.AccessionID == id && r.Resolution != "" {
			return r.AfterMetric
		}
	}
	var g, t float64
	for _, o := range b.Observations {
		if o.AccessionID == id {
			g += float64(o.GerminatedCount)
			t += float64(o.GerminatedCount + o.DormantCount + o.MoldedCount + o.DeadCount)
		}
	}
	if t == 0 {
		return 0
	}
	return g / t * 100
}
func (b *RejuvenationBatch) LastObservationComplete() bool {
	if b.Protocol == nil {
		return false
	}
	for _, id := range itemIDs(b.Items) {
		p, ok := b.ProtocolFor(id)
		if !ok {
			return false
		}
		for _, day := range p.ObservationDays {
			found := false
			for _, o := range b.Observations {
				if o.AccessionID == id && o.DayIndex == day {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		total := 0
		for _, o := range b.Observations {
			if o.AccessionID == id {
				total += observationTotal(o)
			}
		}
		if total != p.SampleSize {
			return false
		}
	}
	return true
}

func observationTotal(o TrialObservation) int {
	return o.GerminatedCount + o.DormantCount + o.MoldedCount + o.DeadCount
}
func itemIDs(xs []AccessionItem) []string {
	r := make([]string, len(xs))
	for i, x := range xs {
		r[i] = x.AccessionID
	}
	return r
}

func (b *RejuvenationBatch) ProtocolFor(id string) (AccessionProtocol, bool) {
	if b.Protocol == nil {
		return AccessionProtocol{}, false
	}
	for _, p := range b.Protocol.Entries {
		if p.AccessionID == id {
			return p, true
		}
	}
	return AccessionProtocol{}, false
}
