package cross_store_idempotency_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"seedvault/internal/application"
	"seedvault/internal/httpapi"
	"seedvault/internal/persistence"
	"testing"
)

type createdBatch struct {
	BatchID string `json:"batch_id"`
}

func createBatch(t *testing.T, handler http.Handler, title, requestID string) createdBatch {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"title":        title,
		"custodian_id": "custodian",
		"request_id":   requestID,
		"items": []map[string]any{{
			"accession_id":       "A1",
			"source_id":          "SRC-1",
			"taxon_name":         "Glycine max",
			"collection_site":    "site-1",
			"collection_year":    2024,
			"storage_generation": 1,
			"seed_count":         20,
			"baseline_viability": 40,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/batches", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create returned status %d: %s", response.Code, response.Body.String())
	}
	var batch createdBatch
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestRequestIDIsIsolatedAcrossStoreLifecycles(t *testing.T) {
	firstStore := persistence.New(t.TempDir())
	secondStore := persistence.New(t.TempDir())
	firstHandler := httpapi.New(application.New(firstStore)).Handler()
	secondHandler := httpapi.New(application.New(secondStore)).Handler()

	first := createBatch(t, firstHandler, "first store batch", "shared-request-id")
	second := createBatch(t, secondHandler, "second store batch", "shared-request-id")

	if first.BatchID == second.BatchID {
		t.Fatalf("second store replayed batch %s owned by the first store", second.BatchID)
	}
	if _, ok := secondStore.Get(second.BatchID); !ok {
		t.Fatalf("second store returned batch %s without persisting it", second.BatchID)
	}
}
