package lsp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	FilePath string `json:"-"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type DiagnosticCounts struct {
	Error       int
	Warning     int
	Information int
	Hint        int
}

type VersionedMap[K comparable, V any] struct {
	mu      sync.RWMutex
	data    map[K]V
	version atomic.Uint64
}

func NewVersionedMap[K comparable, V any]() *VersionedMap[K, V] {
	return &VersionedMap[K, V]{data: make(map[K]V)}
}

func (m *VersionedMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	m.version.Add(1)
}

func (m *VersionedMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.data[key]
	return value, ok
}

func (m *VersionedMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	m.version.Add(1)
}

func (m *VersionedMap[K, V]) Range(fn func(K, V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for key, value := range m.data {
		if !fn(key, value) {
			return
		}
	}
}

func (m *VersionedMap[K, V]) Version() uint64 {
	return m.version.Load()
}

func HandleDiagnostics(client *Client, params json.RawMessage) {
	var diagParams PublishDiagnosticsParams
	if err := json.Unmarshal(params, &diagParams); err != nil {
		return
	}

	filePath := pathFromURI(diagParams.URI)
	for i := range diagParams.Diagnostics {
		diagParams.Diagnostics[i].FilePath = filePath
	}
	client.diagnostics.Set(diagParams.URI, diagParams.Diagnostics)

	if client.onDiagnosticsChanged != nil {
		client.onDiagnosticsChanged(client.name, client.totalDiagnostics())
	}
}

func (c *Client) GetDiagnosticCounts() DiagnosticCounts {
	currentVersion := c.diagnostics.Version()

	c.diagCountsMu.Lock()
	defer c.diagCountsMu.Unlock()
	if currentVersion == c.diagCountsVersion {
		return c.diagCountsCache
	}

	counts := DiagnosticCounts{}
	c.diagnostics.Range(func(_ string, diags []Diagnostic) bool {
		for _, diag := range diags {
			switch diag.Severity {
			case SeverityError:
				counts.Error++
			case SeverityWarning:
				counts.Warning++
			case SeverityInformation:
				counts.Information++
			case SeverityHint:
				counts.Hint++
			default:
				counts.Error++
			}
		}
		return true
	})

	c.diagCountsCache = counts
	c.diagCountsVersion = currentVersion
	return counts
}

func (c *Client) DiagnosticsForFile(filePath string) []Diagnostic {
	uri := uriFromPath(filePath)
	diagnostics, ok := c.diagnostics.Get(uri)
	if !ok {
		return nil
	}
	return append([]Diagnostic(nil), diagnostics...)
}

func (c *Client) WaitForDiagnostics(ctx context.Context, filePath string, afterVersion uint64, timeout time.Duration) {
	uri := uriFromPath(filePath)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, ok := c.diagnostics.Get(uri); ok && c.diagnostics.Version() > afterVersion {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) totalDiagnostics() int {
	total := 0
	c.diagnostics.Range(func(_ string, diags []Diagnostic) bool {
		total += len(diags)
		return true
	})
	return total
}

func uriFromPath(filePath string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	return "file://" + filepath.ToSlash(abs)
}

func pathFromURI(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if path == uri {
		return uri
	}
	return filepath.FromSlash(path)
}
