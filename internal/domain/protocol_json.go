package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func (p *AccessionProtocol) UnmarshalJSON(data []byte) error {
	allowed := map[string]bool{"accession_id": true, "sample_size": true, "treatment_groups": true, "group_allocations": true, "temperature_bounds": true, "humidity_bounds": true, "observation_days": true, "pass_threshold": true}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	type wire struct {
		AccessionID       string                `json:"accession_id"`
		SampleSize        int                   `json:"sample_size"`
		GroupAllocations  []TreatmentAllocation `json:"group_allocations"`
		TemperatureBounds [2]float64            `json:"temperature_bounds"`
		HumidityBounds    [2]float64            `json:"humidity_bounds"`
		ObservationDays   []int                 `json:"observation_days"`
		PassThreshold     float64               `json:"pass_threshold"`
	}
	var decoded wire
	withoutGroups := map[string]json.RawMessage{}
	for key, value := range fields {
		if key != "treatment_groups" {
			withoutGroups[key] = value
		}
	}
	raw, _ := json.Marshal(withoutGroups)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	p.AccessionID, p.SampleSize = decoded.AccessionID, decoded.SampleSize
	p.TemperatureBounds, p.HumidityBounds = decoded.TemperatureBounds, decoded.HumidityBounds
	p.ObservationDays, p.PassThreshold = decoded.ObservationDays, decoded.PassThreshold
	p.TreatmentGroups = append([]TreatmentAllocation(nil), decoded.GroupAllocations...)
	groups := bytes.TrimSpace(fields["treatment_groups"])
	if len(groups) > 0 && string(groups) != "null" {
		var allocations []TreatmentAllocation
		if err := json.Unmarshal(groups, &allocations); err == nil {
			p.TreatmentGroups = allocations
			return nil
		}
		var names []string
		if err := json.Unmarshal(groups, &names); err != nil {
			return err
		}
		if len(p.TreatmentGroups) == 0 && len(names) > 0 {
			share := 0
			if p.SampleSize%len(names) == 0 {
				share = p.SampleSize / len(names)
			}
			for _, name := range names {
				p.TreatmentGroups = append(p.TreatmentGroups, TreatmentAllocation{Name: name, SampleSize: share})
			}
		}
	}
	return nil
}
