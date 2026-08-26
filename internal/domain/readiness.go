package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func DiagnoseDraft(items []AccessionItem) DraftReadiness {
	r := DraftReadiness{ItemCount: len(items)}
	ids := map[string]int{}
	sources := map[string]int{}
	for i, item := range items {
		id := strings.TrimSpace(item.AccessionID)
		if item.SeedCount > 0 {
			r.TotalSeedCount += item.SeedCount
		}
		if item.BaselineViabilityProvided && item.BaselineViability >= 0 && item.BaselineViability < 50 {
			r.LowViabilityCount++
		}
		missing := func(field, value string) {
			if strings.TrimSpace(value) == "" {
				r.MissingFieldCount++
				addDraftDiagnostic(&r, id, field, "field_required", "error", "字段不能为空")
			}
		}
		missing("accession_id", item.AccessionID)
		missing("taxon_name", item.TaxonName)
		missing("collection_site", item.CollectionSite)
		missing("source_id", item.SourceID)
		if item.CollectionYear == 0 {
			r.MissingFieldCount++
			addDraftDiagnostic(&r, id, "collection_year", "field_required", "error", "采集年份不能为空")
		} else if item.CollectionYear < 1800 || item.CollectionYear > time.Now().Year() {
			addDraftDiagnostic(&r, id, "collection_year", "collection_year_range", "error", "采集年份超出有效范围")
		}
		if item.StorageGeneration <= 0 {
			r.MissingFieldCount++
			addDraftDiagnostic(&r, id, "storage_generation", "field_required", "error", "保藏世代必须为正整数")
		}
		if item.SeedCount <= 0 {
			r.MissingFieldCount++
			addDraftDiagnostic(&r, id, "seed_count", "seed_count_positive", "error", "种子数量必须为正整数")
		}
		if !item.BaselineViabilityProvided {
			r.MissingFieldCount++
			addDraftDiagnostic(&r, id, "baseline_viability", "field_required", "error", "基线活力不能为空")
		} else if math.IsNaN(item.BaselineViability) || math.IsInf(item.BaselineViability, 0) || item.BaselineViability < 0 || item.BaselineViability > 100 {
			addDraftDiagnostic(&r, id, "baseline_viability", "baseline_viability_range", "error", "基线活力必须在 0 到 100 之间")
		} else if item.BaselineViability < 50 {
			addDraftDiagnostic(&r, id, "baseline_viability", "low_viability", "warning", "基线活力偏低，请确认试验容量")
		}
		if id != "" {
			if first, ok := ids[id]; ok {
				addDraftDiagnostic(&r, id, "accession_id", "duplicate_accession_id", "error", fmt.Sprintf("与第 %d 个条目重复", first+1))
			} else {
				ids[id] = i
			}
		}
		source := strings.TrimSpace(item.SourceID)
		if source != "" {
			if first, ok := sources[source]; ok {
				prev := items[first]
				addDraftDiagnostic(&r, id, "source_id", "duplicate_source_id", "error", fmt.Sprintf("与 accession_id=%s 使用相同来源标识", prev.AccessionID))
				if prev.TaxonName != item.TaxonName || prev.CollectionSite != item.CollectionSite || prev.CollectionYear != item.CollectionYear {
					addDraftDiagnostic(&r, id, "collection_information", "contradictory_collection_information", "error", "相同来源标识的分类名或采集信息互相矛盾")
				}
			} else {
				sources[source] = i
			}
		}
	}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		if a.AccessionID != b.AccessionID {
			return a.AccessionID < b.AccessionID
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Code < b.Code
	})
	r.Ready = r.BlockingCount == 0 && r.ItemCount > 0
	return r
}

func addDraftDiagnostic(r *DraftReadiness, id, field, code, severity, message string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{AccessionID: id, Field: field, Code: code, Severity: severity, Message: message})
	if severity == "error" {
		r.BlockingCount++
	}
}
