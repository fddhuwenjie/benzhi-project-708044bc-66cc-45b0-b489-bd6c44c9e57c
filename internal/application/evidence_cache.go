package application

import (
	"encoding/json"
	"sync"
)

// evidenceResponseCache avoids rebuilding canonical evidence views for
// repeated archive downloads. Entries are keyed by batch and segment, and
// deep-copied on store and load so callers cannot mutate cached responses.
type evidenceResponseCache struct {
	mu      sync.RWMutex
	entries map[string]map[string]any
}

func newEvidenceResponseCache() *evidenceResponseCache {
	return &evidenceResponseCache{entries: make(map[string]map[string]any)}
}

func (c *evidenceResponseCache) load(selector string) (map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	response, ok := c.entries[selector]
	if !ok {
		return nil, false
	}
	return cloneResponse(response), true
}

func (c *evidenceResponseCache) store(selector string, response map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[selector] = cloneResponse(response)
}

// cloneResponse returns a deep copy of a response map so that mutations by
// callers never leak into the cache or subsequent responses.
func cloneResponse(response map[string]any) map[string]any {
	if response == nil {
		return nil
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil
	}
	var copy map[string]any
	if err := json.Unmarshal(raw, &copy); err != nil {
		return nil
	}
	return copy
}
