package subagent

import (
	"sync"
	"time"
)

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

type ToolCache struct {
	mu         sync.RWMutex
	entries    map[string]cacheEntry
	readOnly   map[string]bool
	defaultTTL time.Duration
}

func NewToolCache(defaultTTL time.Duration, readOnlyTools ...string) *ToolCache {
	if defaultTTL <= 0 {
		defaultTTL = time.Minute
	}
	readOnly := make(map[string]bool)
	for _, tool := range readOnlyTools {
		readOnly[tool] = true
	}
	if len(readOnly) == 0 {
		readOnly["view"] = true
		readOnly["grep"] = true
	}
	return &ToolCache{entries: make(map[string]cacheEntry), readOnly: readOnly, defaultTTL: defaultTTL}
}

func (c *ToolCache) Get(toolName, key string) (string, bool) {
	if c == nil || !c.readOnly[toolName] {
		return "", false
	}
	cacheKey := toolName + ":" + key
	c.mu.RLock()
	entry, ok := c.entries[cacheKey]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			c.mu.Lock()
			delete(c.entries, cacheKey)
			c.mu.Unlock()
		}
		return "", false
	}
	return entry.value, true
}

func (c *ToolCache) Set(toolName, key, value string, ttl time.Duration) bool {
	if c == nil || !c.readOnly[toolName] {
		return false
	}
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	cacheKey := toolName + ":" + key
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey] = cacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
	return true
}
