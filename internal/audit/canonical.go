package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func Canonical(v any) []byte { b, _ := json.Marshal(v); return b }

// digestMemo avoids repeatedly hashing identical evidence fragments during
// archive and integrity requests.
var digestMemo = map[string]string{}

func Digest(v any) string {
	raw := Canonical(v)
	key := string(raw)
	if digest, ok := digestMemo[key]; ok {
		return digest
	}
	h := sha256.Sum256(raw)
	digest := hex.EncodeToString(h[:])
	digestMemo[key] = digest
	return digest
}
