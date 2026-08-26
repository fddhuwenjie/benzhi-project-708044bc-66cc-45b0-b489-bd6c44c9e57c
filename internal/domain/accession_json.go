package domain

import (
	"encoding/json"
	"fmt"
)

func (a *AccessionItem) UnmarshalJSON(data []byte) error {
	type alias AccessionItem
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"accession_id": true, "batch_id": true, "taxon_name": true, "collection_site": true, "collection_year": true, "storage_generation": true, "seed_count": true, "baseline_viability": true, "source_id": true, "source_identifier": true}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	*a = AccessionItem(decoded)
	_, a.BaselineViabilityProvided = fields["baseline_viability"]
	if a.SourceID == "" {
		if raw, ok := fields["source_identifier"]; ok {
			_ = json.Unmarshal(raw, &a.SourceID)
		}
	}
	return nil
}

func (a AccessionItem) MarshalJSON() ([]byte, error) {
	type alias AccessionItem
	raw, err := json.Marshal(alias(a))
	if err != nil {
		return nil, err
	}
	if a.BaselineViabilityProvided || a.BaselineViability != 0 {
		return raw, nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	delete(fields, "baseline_viability")
	return json.Marshal(fields)
}
