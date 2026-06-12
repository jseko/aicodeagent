package lsp

import (
	"encoding/json"
	"testing"
)

func TestVersionedMap(t *testing.T) {
	m := NewVersionedMap[string, int]()
	if got := m.Version(); got != 0 {
		t.Fatalf("initial version = %d", got)
	}
	m.Set("a", 1)
	if got := m.Version(); got != 1 {
		t.Fatalf("version after set = %d", got)
	}
	value, ok := m.Get("a")
	if !ok || value != 1 {
		t.Fatalf("Get returned (%d, %v)", value, ok)
	}
}

func TestHandleDiagnosticsAndCounts(t *testing.T) {
	client := &Client{name: "test", diagnostics: NewVersionedMap[string, []Diagnostic]()}
	params := PublishDiagnosticsParams{
		URI: "file:///tmp/test.go",
		Diagnostics: []Diagnostic{
			{Severity: SeverityError, Message: "undefined"},
			{Severity: SeverityWarning, Message: "unused"},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	HandleDiagnostics(client, data)
	counts := client.GetDiagnosticCounts()
	if counts.Error != 1 || counts.Warning != 1 {
		t.Fatalf("counts = %+v", counts)
	}

	diagnostics := client.DiagnosticsForFile("/tmp/test.go")
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics len = %d", len(diagnostics))
	}
}
