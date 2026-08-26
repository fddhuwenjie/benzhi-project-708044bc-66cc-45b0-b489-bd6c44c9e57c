package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func Canonical(v any) []byte { b, _ := json.Marshal(v); return b }

func Digest(v any) string {
	h := sha256.Sum256(Canonical(v))
	return hex.EncodeToString(h[:])
}
