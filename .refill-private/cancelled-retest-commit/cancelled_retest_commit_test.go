package cancelled_retest_commit_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"strings"
	"syscall"
	"testing"
)

func TestCancelledRetestDoesNotCommit(t *testing.T) {
	dir := t.TempDir()
	store := persistence.New(dir)
	batch := &domain.RejuvenationBatch{
		BatchID: "batch-cancel", State: domain.StateRemediationActive,
		Items: []domain.AccessionItem{{AccessionID: "ACC-1", BatchID: "batch-cancel"}},
		Protocol: &domain.GerminationProtocol{Entries: []domain.AccessionProtocol{{
			AccessionID: "ACC-1", SampleSize: 10, PassThreshold: 70,
		}}},
		Remediations: []domain.RemediationCase{{
			CaseID: "case-1", BatchID: "batch-cancel", AccessionID: "ACC-1",
			BeforeMetric: 40, TreatmentPlan: strings.Repeat("x", 2<<20),
		}},
	}
	if _, err := store.Save(batch, "remediation_started", batch.Remediations[0], -1, ""); err != nil {
		t.Fatal(err)
	}

	fifo := filepath.Join(dir, "batches.json.tmp")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := application.New(store).RetestCountsContext(ctx, batch.BatchID, "case-1", domain.RetestResult{
			SampleSize: 10, GerminatedCount: 8, DeadCount: 2,
		}, "复测通过", "cancel-request", batch.Revision)
		result <- err
	}()

	reader, err := os.Open(fifo)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		reader.Close()
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}

	got, ok := store.Get(batch.BatchID)
	if !ok {
		t.Fatal("batch disappeared")
	}
	if got.Revision != batch.Revision || got.Remediations[0].Resolution != "" {
		t.Fatalf("canceled retest committed revision=%d resolution=%q", got.Revision, got.Remediations[0].Resolution)
	}
	if _, ok := store.IdemBatch("retest:" + batch.BatchID + ":cancel-request"); ok {
		t.Fatal("canceled retest left an idempotency response")
	}
}
