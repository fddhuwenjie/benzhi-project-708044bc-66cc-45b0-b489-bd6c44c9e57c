package domain

import (
	"math"
	"sort"
	"strings"
)

func (p GerminationProtocol) Valid() bool {
	if p.SampleSize <= 0 || len(p.ObservationDays) == 0 || p.PassThreshold < 0 || p.PassThreshold > 100 {
		return false
	}
	if !finiteBounds(p.TemperatureBounds) || !finiteBounds(p.HumidityBounds) || p.TemperatureBounds[0] > p.TemperatureBounds[1] || p.HumidityBounds[0] > p.HumidityBounds[1] {
		return false
	}
	for i := 1; i < len(p.ObservationDays); i++ {
		if p.ObservationDays[i] <= p.ObservationDays[i-1] {
			return false
		}
	}
	return true
}

func PreflightProtocol(items []AccessionItem, p GerminationProtocol) []ProtocolCheck {
	stock := map[string]int{}
	for _, item := range items {
		stock[item.AccessionID] = item.SeedCount
	}
	seen := map[string]bool{}
	checks := make([]ProtocolCheck, 0, len(p.Entries))
	for _, entry := range p.Entries {
		c := ProtocolCheck{AccessionID: entry.AccessionID, Executable: true}
		add := func(field, code, message string) {
			c.Executable = false
			c.Diagnostics = append(c.Diagnostics, Diagnostic{AccessionID: entry.AccessionID, Field: field, Code: code, Severity: "error", Message: message})
		}
		capacity, ok := stock[entry.AccessionID]
		if !ok {
			add("accession_id", "unknown_accession", "方案引用了未知 accession_id")
		}
		if seen[entry.AccessionID] {
			add("accession_id", "duplicate_accession_protocol", "同一条目存在重复方案")
		}
		seen[entry.AccessionID] = true
		if entry.SampleSize <= 0 {
			add("sample_size", "sample_size_positive", "样本量必须为正整数")
		}
		if ok && entry.SampleSize > capacity {
			add("sample_size", "sample_size_exceeds_seed", "样本量超过该条目种子库存")
		}
		groups := map[string]bool{}
		total := 0
		if len(entry.TreatmentGroups) == 0 {
			add("treatment_groups", "treatment_groups_required", "至少需要一个处理组")
		}
		for _, group := range entry.TreatmentGroups {
			name := strings.TrimSpace(group.Name)
			if name == "" || group.SampleSize <= 0 {
				add("treatment_groups", "treatment_group_invalid", "处理组名称和配额必须有效")
			}
			if groups[name] {
				add("treatment_groups", "duplicate_treatment_group", "处理组名称必须唯一")
			}
			groups[name] = true
			total += group.SampleSize
		}
		if total != entry.SampleSize {
			add("treatment_groups", "sample_allocation_mismatch", "各处理组配额之和必须等于条目样本量")
		}
		if !finiteBounds(entry.TemperatureBounds) || entry.TemperatureBounds[0] > entry.TemperatureBounds[1] {
			add("temperature_bounds", "invalid_temperature_bounds", "温度上下界必须为有限值且顺序正确")
		}
		if !finiteBounds(entry.HumidityBounds) || entry.HumidityBounds[0] > entry.HumidityBounds[1] {
			add("humidity_bounds", "invalid_humidity_bounds", "湿度上下界必须为有限值且顺序正确")
		}
		if len(entry.ObservationDays) == 0 {
			add("observation_days", "observation_days_required", "至少需要一个观察日")
		}
		for i, day := range entry.ObservationDays {
			if day <= 0 || i > 0 && day <= entry.ObservationDays[i-1] {
				add("observation_days", "observation_days_order", "观察日必须为严格递增的正整数")
				break
			}
		}
		if math.IsNaN(entry.PassThreshold) || math.IsInf(entry.PassThreshold, 0) || entry.PassThreshold < 0 || entry.PassThreshold > 100 {
			add("pass_threshold", "pass_threshold_range", "通过阈值必须在 0 到 100 之间")
		}
		sort.SliceStable(c.Diagnostics, func(i, j int) bool { return c.Diagnostics[i].Code < c.Diagnostics[j].Code })
		checks = append(checks, c)
	}
	for _, item := range items {
		if !seen[item.AccessionID] {
			checks = append(checks, ProtocolCheck{AccessionID: item.AccessionID, Executable: false, Diagnostics: []Diagnostic{{AccessionID: item.AccessionID, Field: "entries", Code: "accession_protocol_missing", Severity: "error", Message: "缺少该条目的萌发方案"}}})
		}
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].AccessionID < checks[j].AccessionID })
	return checks
}

func ProtocolExecutable(checks []ProtocolCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Executable {
			return false
		}
	}
	return true
}

func finiteBounds(v [2]float64) bool {
	return !math.IsNaN(v[0]) && !math.IsNaN(v[1]) && !math.IsInf(v[0], 0) && !math.IsInf(v[1], 0)
}
