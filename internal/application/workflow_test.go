package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"testing"
)

func TestExpandedWorkflowAtomicityAndIntegrity(t *testing.T) {
	dir := t.TempDir()
	app := New(persistence.New(dir))
	items := []domain.AccessionItem{
		{AccessionID: "A1", SourceID: "S1", TaxonName: "Taxon alpha", CollectionSite: "Site 1", CollectionYear: 2020, StorageGeneration: 2, SeedCount: 20, BaselineViability: 40},
		{AccessionID: "A2", SourceID: "S2", TaxonName: "Taxon beta", CollectionSite: "Site 2", CollectionYear: 2021, StorageGeneration: 1, SeedCount: 20, BaselineViability: 80},
	}
	b, err := app.Create("扩展流程", "custodian", items)
	if err != nil || !b.Readiness.Ready {
		t.Fatalf("create: %v readiness=%+v", err, b.Readiness)
	}
	evidence := make([]domain.IdentityEvidenceItem, 0, len(items))
	for _, item := range items {
		evidence = append(evidence, domain.IdentityEvidenceItem{
			AccessionID:             item.AccessionID,
			CollectionRecord:        domain.EvidenceDocument{EvidenceID: "C-" + item.AccessionID, SourceRef: "collection-log", ClaimedValue: map[string]any{"collection_site": item.CollectionSite, "collection_year": item.CollectionYear}},
			TaxonomicIdentification: domain.EvidenceDocument{EvidenceID: "T-" + item.AccessionID, SourceRef: "taxonomy-log", ClaimedValue: item.TaxonName},
			StorageHistory:          domain.EvidenceDocument{EvidenceID: "H-" + item.AccessionID, SourceRef: "storage-log", ClaimedValue: item.StorageGeneration},
		})
	}
	b, err = app.IdentityStructured(b.BatchID, evidence, []string{"custodian", "identifier"}, b.Revision)
	if err != nil {
		t.Fatal(err)
	}
	protocol := domain.GerminationProtocol{Entries: []domain.AccessionProtocol{
		{AccessionID: "A1", SampleSize: 10, TreatmentGroups: []domain.TreatmentAllocation{{Name: "control", SampleSize: 10}}, TemperatureBounds: [2]float64{20, 25}, HumidityBounds: [2]float64{40, 70}, ObservationDays: []int{1, 2}, PassThreshold: 90},
		{AccessionID: "A2", SampleSize: 10, TreatmentGroups: []domain.TreatmentAllocation{{Name: "control", SampleSize: 10}}, TemperatureBounds: [2]float64{20, 25}, HumidityBounds: [2]float64{40, 70}, ObservationDays: []int{1, 2}, PassThreshold: 80},
	}}
	b, err = app.Protocol(b.BatchID, protocol, b.Revision)
	if err != nil {
		t.Fatal(err)
	}
	before := b.Revision
	_, err = app.ObserveBatch(b.BatchID, []domain.TrialObservation{
		{AccessionID: "A1", ObservedBy: "tech", DayIndex: 1, GerminatedCount: 2, DormantCount: 1},
		{AccessionID: "A2", ObservedBy: "tech", DayIndex: 1, GerminatedCount: 11},
	}, before)
	if err == nil {
		t.Fatal("invalid batch observation accepted")
	}
	b, _ = app.Get(b.BatchID)
	if b.Revision != before || len(b.Observations) != 0 {
		t.Fatalf("partial write: revision=%d observations=%d", b.Revision, len(b.Observations))
	}
	first, err := app.ObserveBatch(b.BatchID, []domain.TrialObservation{
		{AccessionID: "A1", ObservedBy: "tech", DayIndex: 1, GerminatedCount: 2, DormantCount: 1},
		{AccessionID: "A2", ObservedBy: "tech", DayIndex: 1, GerminatedCount: 3},
	}, b.Revision)
	if err != nil {
		t.Fatal(err)
	}
	b, err = app.CorrectObservation(b.BatchID, first.Observations[0].ObservationID, domain.TrialObservation{GerminatedCount: 3, DormantCount: 1}, "修正录入", "tech-2", first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if b.Observations[0].CorrectionCount != 1 {
		t.Fatal("correction metadata missing")
	}
	last, err := app.ObserveBatch(b.BatchID, []domain.TrialObservation{
		{AccessionID: "A1", ObservedBy: "tech", DayIndex: 2, GerminatedCount: 5, DeadCount: 1},
		{AccessionID: "A2", ObservedBy: "tech", DayIndex: 2, GerminatedCount: 5, DeadCount: 2},
	}, b.Revision)
	if err != nil || last.State != domain.StateTrialReview || last.CompletionPercent != 100 {
		t.Fatalf("last observation: %+v %v", last, err)
	}
	b, err = app.Remediate(b.BatchID, domain.RemediationCase{AccessionID: "A1", ReasonCode: "below_threshold", EvidenceRefs: []string{first.Observations[0].ObservationID}, TreatmentPlan: "复壮", RetestProtocolID: "RP-1"}, last.Revision)
	if err != nil {
		t.Fatal(err)
	}
	caseID := b.Remediations[0].CaseID
	b, err = app.RetestCounts(b.BatchID, caseID, domain.RetestResult{SampleSize: 10, GerminatedCount: 9, DeadCount: 1}, "完成", "request-1", b.Revision)
	if err != nil || b.Remediations[0].Retest.Effect != "threshold_met" {
		t.Fatalf("retest: %v %+v", err, b.Remediations[0].Retest)
	}
	replay, err := app.RetestCounts(b.BatchID, caseID, domain.RetestResult{}, "ignored", "request-1", -1)
	if err != nil || replay.Revision != b.Revision {
		t.Fatalf("idempotency replay: %v", err)
	}
	checks := make([]domain.ReviewCheck, 0, len(domain.ReviewCategories))
	for _, category := range domain.ReviewCategories {
		checks = append(checks, domain.ReviewCheck{Category: category, Conclusion: "pass", EvidenceRefs: []string{"aggregate:" + category}, Comment: "核对通过"})
	}
	b, err = app.ReviewChecklist(b.BatchID, "reviewer", "科学审核通过", "approve", checks, b.Revision)
	if err != nil {
		t.Fatal(err)
	}
	b, err = app.Archive(b.BatchID, "A", "2028-01-01", b.Revision)
	if err != nil {
		t.Fatal(err)
	}
	integrity, err := app.DiagnoseIntegrity(b.BatchID)
	if err != nil || !integrity.Valid {
		t.Fatalf("integrity: %v %+v", err, integrity)
	}
	segment, err := app.EvidencePackage(b.BatchID, "observation")
	if err != nil || segment["package_id"] == nil || segment["segment_digest"] == nil {
		t.Fatalf("segment: %v %+v", err, segment)
	}
	reloaded := New(persistence.New(dir))
	if result, reloadErr := reloaded.DiagnoseIntegrity(b.BatchID); reloadErr != nil || !result.Valid {
		t.Fatalf("reloaded integrity: %v %+v", reloadErr, result)
	}
	path := filepath.Join(dir, "batches.json")
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var snapshots map[string]*domain.RejuvenationBatch
	if err := json.Unmarshal(raw, &snapshots); err != nil {
		t.Fatal(err)
	}
	snapshots[b.BatchID].Observations[0].GerminatedCount++
	snapshots[b.BatchID].Observations[0].DormantCount--
	raw, _ = json.Marshal(snapshots)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	tampered, tamperErr := New(persistence.New(dir)).DiagnoseIntegrity(b.BatchID)
	if tamperErr != nil || tampered.Valid || tampered.FirstBrokenSegment != "observation" || tampered.FirstBrokenRevision == 0 || tampered.ManifestDigestValid {
		t.Fatalf("tamper diagnosis: %v %+v", tamperErr, tampered)
	}
}

func TestDraftDiagnosticsAreReadOnlyAndStable(t *testing.T) {
	app := New(persistence.New(""))
	b, err := app.Create("草稿", "custodian", []domain.AccessionItem{
		{AccessionID: "A2", SourceID: "SRC", CollectionSite: "X", CollectionYear: 2020, StorageGeneration: 1, SeedCount: 5, BaselineViability: 101},
		{AccessionID: "A1", SourceID: "SRC", TaxonName: "T", CollectionSite: "Y", CollectionYear: 2021, StorageGeneration: 1, SeedCount: 5, BaselineViability: 20},
	})
	if err != nil || b.Readiness.Ready {
		t.Fatalf("unexpected readiness: %v %+v", err, b.Readiness)
	}
	revision, eventCount := b.Revision, len(app.Store.Events(b.BatchID))
	first, _ := app.Get(b.BatchID)
	second, _ := app.Get(b.BatchID)
	if first.Revision != revision || second.Revision != revision || len(app.Store.Events(b.BatchID)) != eventCount {
		t.Fatal("readiness query changed aggregate")
	}
	if len(first.Readiness.Diagnostics) == 0 || first.Readiness.Diagnostics[0] != second.Readiness.Diagnostics[0] {
		t.Fatal("diagnostics are not stable")
	}
}
