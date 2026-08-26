package application

import "sync"

// evidenceResponseCache avoids rebuilding canonical evidence views for
// repeated archive downloads.
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
	return response, ok
}

func (c *evidenceResponseCache) store(selector string, response map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[selector] = response
}
