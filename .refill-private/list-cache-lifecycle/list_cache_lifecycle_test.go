package listcachelifecycle_test

import (
	"testing"

	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
)

func TestListCacheIsolatedAcrossCommitLifecycle(t *testing.T) {
	store := persistence.New("")
	app := application.New(store)
	batch := &domain.RejuvenationBatch{
		BatchID:     "batch-cache-lifecycle",
		Title:       "initial title",
		CustodianID: "custodian-1",
		State:       domain.StateDraft,
	}
	if _, err := store.Save(batch, "batch_created", map[string]any{"title": batch.Title}, -1, ""); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	first := app.List()
	if len(first) != 1 {
		t.Fatalf("want one cached batch, got %d", len(first))
	}
	first[0].Title = "caller-owned mutation"
	mutationLeaked := app.List()[0].Title == "caller-owned mutation"

	batch.Title = "committed title"
	batch.State = domain.StateIdentityVerified
	if _, err := store.Save(batch, "identity_verified", map[string]any{"confirmed": true}, batch.Revision, ""); err != nil {
		t.Fatalf("commit state transition: %v", err)
	}
	detail, ok := store.Get(batch.BatchID)
	if !ok || detail.Title != "committed title" || detail.State != domain.StateIdentityVerified || detail.Revision != 2 {
		t.Fatalf("detail query did not expose committed state: %+v", detail)
	}

	listed := app.List()[0]
	staleAfterCommit := listed.Title != detail.Title || listed.State != detail.State || listed.Revision != detail.Revision
	if mutationLeaked || staleAfterCommit {
		t.Fatalf("list snapshot crossed caller or commit lifecycle: mutation_leaked=%t stale_after_commit=%t listed=%+v detail=%+v", mutationLeaked, staleAfterCommit, listed, detail)
	}
}
