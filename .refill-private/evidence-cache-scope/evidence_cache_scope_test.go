package evidence_cache_scope_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"seedvault/internal/httpapi"
	"seedvault/internal/persistence"
	"testing"
)

func TestEvidencePackageCacheIsScopedAndOwned(t *testing.T) {
	store := persistence.New("")
	seedArchivedBatch(t, store, "batch-a", "package-a")
	seedArchivedBatch(t, store, "batch-b", "package-b")

	app := application.New(store)
	handler := httpapi.New(app).Handler()
	first, err := app.EvidencePackage("batch-a", "identity")
	if err != nil {
		t.Fatalf("prime batch-a evidence cache: %v", err)
	}
	first["caller_marker"] = "polluted"

	againA := getEvidence(t, handler, "batch-a")
	if marker, exists := againA["caller_marker"]; exists {
		t.Errorf("batch-a cache retained caller-owned mutation: caller_marker=%v", marker)
	}

	batchB := getEvidence(t, handler, "batch-b")
	if got := batchB["package_id"]; got != "package-b" {
		t.Errorf("batch-b received another batch's cached evidence: package_id=%v", got)
	}
}

func seedArchivedBatch(t *testing.T, store *persistence.Store, batchID, packageID string) {
	t.Helper()
	batch := &domain.RejuvenationBatch{
		BatchID: batchID,
		Title:   batchID,
		State:   domain.StateArchived,
		Release: &domain.ReleaseEvidencePackage{
			PackageID: packageID,
			BatchID:   batchID,
			Segments:  []domain.SegmentDigest{{Name: "identity", Digest: "digest-" + batchID}},
		},
	}
	if _, err := store.Save(batch, "archived", batch.Release, -1, ""); err != nil {
		t.Fatalf("seed %s: %v", batchID, err)
	}
}

func getEvidence(t *testing.T, handler http.Handler, batchID string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+batchID+"/evidence-package?segment=identity", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET evidence for %s: status=%d body=%s", batchID, response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode evidence for %s: %v", batchID, err)
	}
	return body
}
