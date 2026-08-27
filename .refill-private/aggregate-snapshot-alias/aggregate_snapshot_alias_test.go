package aggregate_snapshot_alias_test

import (
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"testing"
)

func TestRejectedRetestCannotMutateStoredAggregate(t *testing.T) {
	app := application.New(persistence.New(""))
	batch, err := app.Create("快照所有权复现", "custodian", []domain.AccessionItem{{
		AccessionID:       "A1",
		SourceID:          "SRC-1",
		TaxonName:         "Taxon alpha",
		CollectionSite:    "Site 1",
		CollectionYear:    2020,
		StorageGeneration: 1,
		SeedCount:         10,
		BaselineViability: 40,
	}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []domain.IdentityEvidenceItem{{
		AccessionID: "A1",
		CollectionRecord: domain.EvidenceDocument{
			EvidenceID: "C-A1", SourceRef: "collection-log",
			ClaimedValue: map[string]any{"collection_site": "Site 1", "collection_year": 2020},
		},
		TaxonomicIdentification: domain.EvidenceDocument{EvidenceID: "T-A1", SourceRef: "taxonomy-log", ClaimedValue: "Taxon alpha"},
		StorageHistory:          domain.EvidenceDocument{EvidenceID: "H-A1", SourceRef: "storage-log", ClaimedValue: 1},
	}}
	batch, err = app.IdentityStructured(batch.BatchID, evidence, []string{"custodian", "identifier"}, batch.Revision)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = app.Protocol(batch.BatchID, domain.GerminationProtocol{Entries: []domain.AccessionProtocol{{
		AccessionID:       "A1",
		SampleSize:        10,
		TreatmentGroups:   []domain.TreatmentAllocation{{Name: "control", SampleSize: 10}},
		TemperatureBounds: [2]float64{20, 25},
		HumidityBounds:    [2]float64{40, 70},
		ObservationDays:   []int{1},
		PassThreshold:     70,
	}}}, batch.Revision)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := app.ObserveBatch(batch.BatchID, []domain.TrialObservation{{
		AccessionID: "A1", ObservedBy: "technician", DayIndex: 1,
		GerminatedCount: 4, DeadCount: 6,
	}}, batch.Revision)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = app.Remediate(batch.BatchID, domain.RemediationCase{
		AccessionID: "A1", ReasonCode: "below_threshold", EvidenceRefs: []string{"observation"},
		TreatmentPlan: "rehydration", RetestProtocolID: "RP-1",
	}, observed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	caseID := batch.Remediations[0].CaseID
	_, err = app.RetestCounts(batch.BatchID, caseID, domain.RetestResult{
		SampleSize: 10, GerminatedCount: 8, DeadCount: 2,
	}, "completed", "stale-retest", batch.Revision-1)
	if err == nil || err.Error() != "revision_conflict" {
		t.Fatalf("expected revision_conflict, got %v", err)
	}

	stored, err := app.Get(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	remediation := stored.Remediations[0]
	if stored.State != domain.StateRemediationActive || remediation.Resolution != "" || remediation.Retest != nil {
		t.Fatalf("rejected retest leaked into stored aggregate: state=%s resolution=%q retest=%+v", stored.State, remediation.Resolution, remediation.Retest)
	}
	committed, err := app.RetestCounts(stored.BatchID, caseID, domain.RetestResult{
		SampleSize: 10, GerminatedCount: 8, DeadCount: 2,
	}, "completed", "current-retest", stored.Revision)
	if err != nil || committed.State != domain.StateTrialReview || committed.Remediations[0].Retest == nil {
		t.Fatalf("current revision retest did not commit: batch=%+v err=%v", committed, err)
	}
}
