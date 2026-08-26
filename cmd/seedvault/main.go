package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"seedvault/internal/application"
	"seedvault/internal/httpapi"
	"seedvault/internal/persistence"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	self := flag.Bool("self-check", false, "运行自检")
	flag.Parse()
	if p := os.Getenv("PORT"); p != "" {
		set := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "addr" {
				set = true
			}
		})
		if !set {
			*addr = "127.0.0.1:" + p
		}
	}
	if *self {
		if e := selfCheck(*addr); e != nil {
			panic(e)
		}
		return
	}
	s := persistence.New("data")
	srv := &http.Server{Addr: *addr, Handler: httpapi.New(application.New(s)).Handler(), ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	ln, e := net.Listen("tcp", *addr)
	if e != nil {
		panic(e)
	}
	fmt.Println("seedvault listening", ln.Addr())
	if e = srv.Serve(ln); e != nil && e != http.ErrServerClosed {
		panic(e)
	}
}
func call(base, path string, body any, method string, rev int) (map[string]any, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	q, _ := http.NewRequest(method, base+path, r)
	q.Header.Set("Content-Type", "application/json")
	if rev >= 0 {
		q.Header.Set("X-Expected-Revision", fmt.Sprint(rev))
	}
	resp, e := http.DefaultClient.Do(q)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("status %d: %v", resp.StatusCode, out)
	}
	return out, nil
}
func selfCheck(addr string) error {
	dir, e := os.MkdirTemp("", "seedvault-check-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	a := application.New(persistence.New(filepath.Join(dir, "store")))
	srv := &http.Server{Addr: addr, Handler: httpapi.New(a).Handler()}
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}
	go srv.Serve(ln)
	defer srv.Close()
	time.Sleep(30 * time.Millisecond)
	base := "http://" + addr
	b, e := call(base, "/api/v1/batches", map[string]any{"title": "自检批次", "custodian_id": "c1", "items": []any{map[string]any{"accession_id": "A1", "source_id": "SRC-1", "taxon_name": "豆科", "collection_site": "试验地", "collection_year": 2024, "storage_generation": 1, "seed_count": 10, "baseline_viability": 40}}}, "POST", -1)
	if e != nil {
		return e
	}
	id := b["batch_id"].(string)
	rev := int(b["revision"].(float64))
	b, e = call(base, "/api/v1/batches/"+id+"/identity", map[string]any{"evidence": map[string]any{"A1": map[string]any{
		"collection_record":        map[string]any{"evidence_id": "C-A1", "source_ref": "采集记录", "claimed_value": map[string]any{"collection_site": "试验地", "collection_year": 2024}},
		"taxonomic_identification": map[string]any{"evidence_id": "T-A1", "source_ref": "鉴定记录", "claimed_value": "豆科"},
		"storage_history":          map[string]any{"evidence_id": "H-A1", "source_ref": "保藏记录", "claimed_value": 1},
	}}, "confirmed_by": []string{"c1", "c2"}}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	b, e = call(base, "/api/v1/batches/"+id+"/protocol", map[string]any{"entries": []any{map[string]any{"accession_id": "A1", "sample_size": 10, "treatment_groups": []any{map[string]any{"name": "常规", "sample_size": 10}}, "temperature_bounds": []float64{20, 25}, "humidity_bounds": []float64{40, 70}, "observation_days": []int{1}, "pass_threshold": 70}}}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	b, e = call(base, "/api/v1/batches/"+id+"/observations", map[string]any{"day_index": 1, "observations": []any{map[string]any{"accession_id": "A1", "germinated_count": 4, "dead_count": 6, "observed_by": "t1"}}}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	b, e = call(base, "/api/v1/batches/"+id+"/remediation", map[string]any{"accession_id": "A1", "reason_code": "low_viability", "evidence_refs": []string{"obs-1"}, "treatment_plan": "补水复壮", "retest_protocol_id": "p1"}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	cid := b["remediations"].([]any)[0].(map[string]any)["case_id"].(string)
	b, e = call(base, "/api/v1/batches/"+id+"/retest", map[string]any{"case_id": cid, "sample_size": 10, "germinated_count": 8, "dead_count": 2, "resolution": "通过", "request_id": "self-check-retest"}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	checks := make([]any, 0, 5)
	for _, category := range []string{"identity_chain", "protocol_deviation", "observation_completeness", "threshold_determination", "remediation_closure"} {
		checks = append(checks, map[string]any{"category": category, "conclusion": "pass", "evidence_refs": []string{"aggregate:" + category}, "comment": "核对通过"})
	}
	b, e = call(base, "/api/v1/batches/"+id+"/review", map[string]any{"reviewer_id": "r1", "summary": "审核通过", "decision": "approve", "checks": checks}, "POST", rev)
	if e != nil {
		return e
	}
	rev = int(b["revision"].(float64))
	if _, e = call(base, "/api/v1/batches/"+id+"/archive", map[string]any{"release_grade": "A", "recheck_due_date": "2027-01-01"}, "POST", rev); e != nil {
		return e
	}
	if _, e = call(base, "/api/v1/batches/"+id+"/integrity", nil, "GET", -1); e != nil {
		return e
	}
	fmt.Println("self-check ok")
	return nil
}
