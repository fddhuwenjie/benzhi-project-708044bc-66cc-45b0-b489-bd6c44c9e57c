package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"seedvault/internal/domain"
	"sort"
)

type Event struct {
	Revision int    `json:"revision"`
	Type     string `json:"type"`
	Payload  any    `json:"payload"`
	PrevHash string `json:"prev_hash,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

func HashEvent(prev string, e Event) string {
	e.Hash = ""
	e.PrevHash = prev
	if raw, err := json.Marshal(e.Payload); err == nil {
		var normalized any
		if err := json.Unmarshal(raw, &normalized); err == nil {
			e.Payload = normalized
		} else {
			e.Payload = fmt.Sprint(e.Payload)
		}
	} else {
		e.Payload = fmt.Sprint(e.Payload)
	}
	b, _ := json.Marshal(e)
	h := sha256.Sum256(append([]byte(prev), b...))
	return hex.EncodeToString(h[:])
}

// normalizePayload detaches the payload from any caller-owned mutable state by
// round-tripping it through JSON. This guarantees that a stored event retains a
// snapshot of the payload as it was at commit time, so later mutations to the
// returned response object cannot alter the committed audit event. The
// transformation is idempotent and matches the normalization HashEvent already
// applies when computing the hash, so existing hashes are unchanged.
func normalizePayload(p any) any {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprint(p)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return fmt.Sprint(p)
	}
	return normalized
}

func Append(events []Event, typ string, rev int, p any) []Event {
	prev := ""
	if len(events) > 0 {
		prev = events[len(events)-1].Hash
	}
	e := Event{Revision: rev, Type: typ, Payload: normalizePayload(p), PrevHash: prev}
	e.Hash = HashEvent(prev, e)
	return append(events, e)
}
func Verify(events []Event) error {
	prev := ""
	for i, e := range events {
		if e.Revision != i+1 {
			return fmt.Errorf("broken_revision_%d", i+1)
		}
		if HashEvent(prev, e) != e.Hash {
			return fmt.Errorf("broken_chain_%d", e.Revision)
		}
		prev = e.Hash
	}
	return nil
}
func Manifest(b *domain.RejuvenationBatch, events []Event) (map[string]any, string) {
	m := map[string]any{"batch_id": b.BatchID, "state": b.State, "revision": b.Revision, "items": b.Items, "protocol": b.Protocol, "observations": b.Observations, "remediations": b.Remediations}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var raw []byte
	for _, k := range keys {
		x, _ := json.Marshal(m[k])
		raw = append(raw, []byte(k+":")...)
		raw = append(raw, x...)
	}
	h := sha256.Sum256(raw)
	return m, hex.EncodeToString(h[:])
}
