package corrupt_snapshot_overwrite_test

import (
	"os"
	"path/filepath"
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"testing"
)

func validItem(id string) domain.AccessionItem {
	return domain.AccessionItem{
		AccessionID: id, SourceID: "source-" + id, TaxonName: "Taxon", CollectionSite: "Site",
		CollectionYear: 2020, StorageGeneration: 1, SeedCount: 10, BaselineViability: 60,
	}
}

func TestCorruptSnapshotPreventsDestructiveWrite(t *testing.T) {
	dir := t.TempDir()
	app := application.New(persistence.New(dir))
	original, err := app.Create("原始批次", "custodian", []domain.AccessionItem{validItem("A1")})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "batches.json")
	if err := os.WriteFile(path, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	reloaded := application.New(persistence.New(dir))
	if _, err := reloaded.Create("不应覆盖", "custodian", []domain.AccessionItem{validItem("A2")}); err == nil {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		t.Fatalf("corrupt snapshot was silently discarded and overwritten; original batch %s, new snapshot %s", original.BatchID, raw)
	}
}
