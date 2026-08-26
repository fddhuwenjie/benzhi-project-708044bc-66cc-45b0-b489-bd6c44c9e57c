package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func BuildIdentityMatrix(items []AccessionItem, evidence []IdentityEvidenceItem) []ConflictMatrixEntry {
	byID := map[string]IdentityEvidenceItem{}
	duplicate := map[string]bool{}
	known := map[string]AccessionItem{}
	for _, item := range items {
		known[item.AccessionID] = item
	}
	for _, e := range evidence {
		if _, ok := byID[e.AccessionID]; ok {
			duplicate[e.AccessionID] = true
		}
		byID[e.AccessionID] = e
	}
	var out []ConflictMatrixEntry
	for _, e := range evidence {
		if _, ok := known[e.AccessionID]; !ok {
			out = append(out, matrix(e.AccessionID, "evidence", "accession_id", "conflict", nil, e.AccessionID, true, "证据引用了未知 accession_id"))
		}
	}
	for _, item := range items {
		e, ok := byID[item.AccessionID]
		if !ok {
			for _, kind := range []string{"collection_record", "taxonomic_identification", "storage_history"} {
				out = append(out, matrix(item.AccessionID, kind, "evidence", "missing", nil, nil, true, "缺少该类证据"))
			}
			continue
		}
		if duplicate[item.AccessionID] {
			out = append(out, matrix(item.AccessionID, "evidence", "accession_id", "conflict", item.AccessionID, item.AccessionID, true, "同一条目提交了重复证据组"))
		}
		out = append(out, validateEvidenceMeta(item.AccessionID, "collection_record", e.CollectionRecord)...)
		out = append(out, validateEvidenceMeta(item.AccessionID, "taxonomic_identification", e.TaxonomicIdentification)...)
		out = append(out, validateEvidenceMeta(item.AccessionID, "storage_history", e.StorageHistory)...)
		site, siteOK := claimString(e.CollectionRecord, "collection_site")
		year, yearOK := claimInt(e.CollectionRecord, "collection_year")
		taxon, taxonOK := claimString(e.TaxonomicIdentification, "taxon_name")
		generation, generationOK := claimInt(e.StorageHistory, "storage_generation")
		out = append(out, compareClaim(item.AccessionID, "collection_record", "collection_site", item.CollectionSite, site, siteOK))
		out = append(out, compareClaim(item.AccessionID, "collection_record", "collection_year", item.CollectionYear, year, yearOK))
		out = append(out, compareClaim(item.AccessionID, "taxonomic_identification", "taxon_name", item.TaxonName, taxon, taxonOK))
		out = append(out, compareClaim(item.AccessionID, "storage_history", "storage_generation", item.StorageGeneration, generation, generationOK))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.AccessionID != b.AccessionID {
			return a.AccessionID < b.AccessionID
		}
		if a.Evidence != b.Evidence {
			return a.Evidence < b.Evidence
		}
		return a.Field < b.Field
	})
	return out
}

func validateEvidenceMeta(id, kind string, d EvidenceDocument) []ConflictMatrixEntry {
	var out []ConflictMatrixEntry
	if strings.TrimSpace(d.EvidenceID) == "" {
		out = append(out, matrix(id, kind, "evidence_id", "missing", nil, nil, true, "证据编号不能为空"))
	}
	if strings.TrimSpace(d.SourceRef) == "" {
		out = append(out, matrix(id, kind, "source_ref", "missing", nil, nil, true, "引用来源不能为空"))
	}
	allowed := map[string]bool{}
	switch kind {
	case "collection_record":
		allowed["collection_site"], allowed["collection_year"] = true, true
		if d.TaxonName != "" || d.Generation != 0 {
			out = append(out, matrix(id, kind, "claimed_value", "conflict", nil, nil, true, "采集记录包含未知声明字段"))
		}
	case "taxonomic_identification":
		allowed["taxon_name"] = true
		if d.CollectionSite != "" || d.CollectionYear != 0 || d.Generation != 0 {
			out = append(out, matrix(id, kind, "claimed_value", "conflict", nil, nil, true, "分类鉴定包含未知声明字段"))
		}
	case "storage_history":
		allowed["storage_generation"] = true
		if d.CollectionSite != "" || d.CollectionYear != 0 || d.TaxonName != "" {
			out = append(out, matrix(id, kind, "claimed_value", "conflict", nil, nil, true, "历史保藏证据包含未知声明字段"))
		}
	}
	for key := range d.Claims {
		if !allowed[key] {
			out = append(out, matrix(id, kind, key, "conflict", nil, nil, true, "未知证据声明字段"))
		}
	}
	if claims, ok := d.ClaimedValue.(map[string]any); ok {
		for key := range claims {
			if !allowed[key] {
				out = append(out, matrix(id, kind, key, "conflict", nil, nil, true, "未知证据声明字段"))
			}
		}
	}
	return out
}

func compareClaim(id, kind, field string, expected, claimed any, present bool) ConflictMatrixEntry {
	if !present || emptyClaim(claimed) {
		return matrix(id, kind, field, "missing", expected, nil, true, "声明值不能为空")
	}
	if !reflect.DeepEqual(normalized(expected), normalized(claimed)) {
		return matrix(id, kind, field, "conflict", expected, claimed, true, "证据声明与建档快照不一致")
	}
	return matrix(id, kind, field, "consistent", expected, claimed, false, "")
}

func matrix(id, evidence, field, result string, expected, claimed any, blocking bool, message string) ConflictMatrixEntry {
	return ConflictMatrixEntry{AccessionID: id, Evidence: evidence, Field: field, Result: result, Expected: expected, Claimed: claimed, Blocking: blocking, Message: message}
}

func claimString(d EvidenceDocument, field string) (string, bool) {
	if field == "collection_site" && strings.TrimSpace(d.CollectionSite) != "" {
		return strings.TrimSpace(d.CollectionSite), true
	}
	if field == "taxon_name" && strings.TrimSpace(d.TaxonName) != "" {
		return strings.TrimSpace(d.TaxonName), true
	}
	if v, ok := lookupClaim(d, field); ok {
		s := strings.TrimSpace(fmt.Sprint(v))
		return s, s != ""
	}
	if s, ok := d.ClaimedValue.(string); ok {
		s = strings.TrimSpace(s)
		return s, s != ""
	}
	return "", false
}

func claimInt(d EvidenceDocument, field string) (int, bool) {
	if field == "collection_year" && d.CollectionYear != 0 {
		return d.CollectionYear, true
	}
	if field == "storage_generation" && d.Generation != 0 {
		return d.Generation, true
	}
	v, ok := lookupClaim(d, field)
	if !ok {
		v = d.ClaimedValue
	}
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), x == float64(int(x))
	case json.Number:
		n, err := strconv.Atoi(x.String())
		return n, err == nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	default:
		return 0, false
	}
}

func lookupClaim(d EvidenceDocument, field string) (any, bool) {
	if d.Claims != nil {
		if v, ok := d.Claims[field]; ok {
			return v, true
		}
	}
	if m, ok := d.ClaimedValue.(map[string]any); ok {
		v, exists := m[field]
		return v, exists
	}
	return nil, false
}

func emptyClaim(v any) bool   { return v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" }
func normalized(v any) string { return strings.TrimSpace(strings.ToLower(fmt.Sprint(v))) }
