package persistence_write_errors_test

import (
	"os"
	"path/filepath"
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"testing"
)

func TestPersistenceWriteFailureIsReturned(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file blocks store directory"), 0600); err != nil {
		t.Fatal(err)
	}
	app := application.New(persistence.New(blocked))
	created, err := app.Create("持久化失败", "custodian", []domain.AccessionItem{{
		AccessionID: "A1", SourceID: "S1", TaxonName: "Taxon", CollectionSite: "Site",
		CollectionYear: 2020, StorageGeneration: 1, SeedCount: 10, BaselineViability: 60,
	}})
	if err == nil {
		if _, reloadErr := application.New(persistence.New(blocked)).Get(created.BatchID); reloadErr == nil {
			t.Fatal("unexpectedly reloaded a batch from an unusable store path")
		}
		t.Fatalf("persistence write failure was reported as a successful create for batch %s", created.BatchID)
	}
}
