package eventpayloadownership

import (
	"seedvault/internal/application"
	"seedvault/internal/audit"
	"seedvault/internal/domain"
	"seedvault/internal/persistence"
	"testing"
)

func TestEventPayloadIsOwnedByAuditLog(t *testing.T) {
	store := persistence.New("")
	app := application.New(store)
	batch, err := app.Create("审计载荷所有权", "custodian-1", []domain.AccessionItem{{
		AccessionID:       "ACC-1",
		SourceID:          "SRC-1",
		TaxonName:         "Taxon original",
		CollectionSite:    "Site-1",
		CollectionYear:    2024,
		StorageGeneration: 1,
		SeedCount:         20,
		BaselineViability: 60,
	}})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := audit.Verify(store.Events(batch.BatchID)); err != nil {
		t.Fatalf("new audit chain is invalid: %v", err)
	}

	// App.Create returns an application-owned aggregate. Mutating that response
	// must not mutate the append-only payload already accepted by Store.Save.
	batch.Items[0].TaxonName = "Taxon changed by caller"

	if err := audit.Verify(store.Events(batch.BatchID)); err != nil {
		t.Fatalf("caller mutation corrupted committed audit chain: %v", err)
	}
	stored, ok := store.Get(batch.BatchID)
	if !ok || stored.Items[0].TaxonName != "Taxon original" {
		t.Fatalf("snapshot unexpectedly changed with caller response: %+v", stored)
	}
}
