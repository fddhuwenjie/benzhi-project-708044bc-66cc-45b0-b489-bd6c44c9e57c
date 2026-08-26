package domain

import (
	"errors"
	"sort"
)

func (b *RejuvenationBatch) ValidateObservationBatch(observations []TrialObservation) error {
	if len(observations) == 0 {
		return errors.New("observations_required")
	}
	day := observations[0].DayIndex
	seen := map[string]bool{}
	temporary := *b
	temporary.Observations = append([]TrialObservation(nil), b.Observations...)
	for _, o := range observations {
		if o.DayIndex != day {
			return RuleError{Code: "mixed_day_index", Detail: o.AccessionID}
		}
		if seen[o.AccessionID] {
			return RuleError{Code: "duplicate_accession_id", Detail: o.AccessionID}
		}
		seen[o.AccessionID] = true
		p, ok := b.ProtocolFor(o.AccessionID)
		if !ok {
			return RuleError{Code: "unknown_accession", Detail: o.AccessionID}
		}
		dayOK := false
		for _, scheduled := range p.ObservationDays {
			if scheduled == day {
				dayOK = true
				break
			}
		}
		if !dayOK {
			return RuleError{Code: "invalid_day_index", Detail: o.AccessionID}
		}
		if err := temporary.ValidateObservation(o); err != nil {
			return RuleError{Code: err.Error(), Detail: o.AccessionID}
		}
		temporary.Observations = append(temporary.Observations, o)
	}
	return nil
}

func (b *RejuvenationBatch) ReplaceObservation(id string, replacement TrialObservation) (TrialObservation, error) {
	index := -1
	for i, o := range b.Observations {
		if o.ObservationID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return TrialObservation{}, errors.New("observation_not_found")
	}
	original := b.Observations[index]
	if replacement.AccessionID != "" && replacement.AccessionID != original.AccessionID {
		return TrialObservation{}, errors.New("correction_accession_immutable")
	}
	if replacement.DayIndex != 0 && replacement.DayIndex != original.DayIndex {
		return TrialObservation{}, errors.New("correction_day_immutable")
	}
	replacement.ObservationID = original.ObservationID
	replacement.BatchID = original.BatchID
	replacement.AccessionID = original.AccessionID
	replacement.DayIndex = original.DayIndex
	replacement.ObservedAt = original.ObservedAt
	if replacement.ObservedBy == "" {
		replacement.ObservedBy = original.ObservedBy
	}
	replacement.CorrectionCount = original.CorrectionCount + 1
	temporary := *b
	temporary.Observations = append([]TrialObservation(nil), b.Observations...)
	temporary.Observations = append(temporary.Observations[:index], temporary.Observations[index+1:]...)
	sort.SliceStable(temporary.Observations, func(i, j int) bool {
		if temporary.Observations[i].AccessionID != temporary.Observations[j].AccessionID {
			return temporary.Observations[i].AccessionID < temporary.Observations[j].AccessionID
		}
		return temporary.Observations[i].DayIndex < temporary.Observations[j].DayIndex
	})
	// Validate the full sequence after replacement, not only the changed row.
	var sequence []TrialObservation
	for _, o := range temporary.Observations {
		if o.AccessionID == original.AccessionID {
			sequence = append(sequence, o)
		}
	}
	sequence = append(sequence, replacement)
	sort.SliceStable(sequence, func(i, j int) bool { return sequence[i].DayIndex < sequence[j].DayIndex })
	check := *b
	check.Observations = nil
	for _, o := range sequence {
		if err := check.ValidateObservation(o); err != nil {
			return TrialObservation{}, err
		}
		check.Observations = append(check.Observations, o)
	}
	b.Observations[index] = replacement
	return original, nil
}

func (b *RejuvenationBatch) ObservationProgress() ([]ObservationProgress, float64, *int) {
	progress := make([]ObservationProgress, 0, len(b.Items))
	completedSlots, totalSlots := 0, 0
	var globalNext *int
	for _, item := range b.Items {
		p, ok := b.ProtocolFor(item.AccessionID)
		if !ok {
			continue
		}
		done := map[int]bool{}
		germinated, total := 0, 0
		for _, o := range b.Observations {
			if o.AccessionID != item.AccessionID {
				continue
			}
			done[o.DayIndex] = true
			germinated += o.GerminatedCount
			total += observationTotal(o)
		}
		totalSlots += len(p.ObservationDays)
		completedSlots += len(done)
		var next *int
		for _, day := range p.ObservationDays {
			if !done[day] {
				d := day
				next = &d
				if globalNext == nil || d < *globalNext {
					g := d
					globalNext = &g
				}
				break
			}
		}
		percent := 100.0
		if len(p.ObservationDays) > 0 {
			percent = float64(len(done)) * 100 / float64(len(p.ObservationDays))
		}
		progress = append(progress, ObservationProgress{AccessionID: item.AccessionID, CumulativeCount: total, CumulativeViability: Viability(germinated, total), CompletionPercent: percent, NextObservationDay: next})
	}
	sort.Slice(progress, func(i, j int) bool { return progress[i].AccessionID < progress[j].AccessionID })
	overall := 0.0
	if totalSlots > 0 {
		overall = float64(completedSlots) * 100 / float64(totalSlots)
	}
	return progress, overall, globalNext
}

func CountsOf(o TrialObservation) Counts {
	return Counts{GerminatedCount: o.GerminatedCount, DormantCount: o.DormantCount, MoldedCount: o.MoldedCount, DeadCount: o.DeadCount}
}
