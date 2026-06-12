package lsp

import (
	"testing"
	"time"
)

func TestCalculateDelta(t *testing.T) {
	before := &DiagnosticSnapshot{TotalErrors: 5, TotalWarnings: 3}
	after := &DiagnosticSnapshot{TotalErrors: 2, TotalWarnings: 4}
	delta := CalculateDelta(before, after)

	if delta.ErrorsFixed != 3 {
		t.Fatalf("ErrorsFixed = %d", delta.ErrorsFixed)
	}
	if delta.WarningsAdded != 1 {
		t.Fatalf("WarningsAdded = %d", delta.WarningsAdded)
	}
	if delta.NetImprovement != 5 {
		t.Fatalf("NetImprovement = %d", delta.NetImprovement)
	}
}

func TestRepairMetrics(t *testing.T) {
	before := &DiagnosticSnapshot{TotalErrors: 4, TotalWarnings: 1}
	after := &DiagnosticSnapshot{TotalErrors: 0, TotalWarnings: 0}
	metrics := CalculateRepairMetrics(before, after, 1, time.Second)

	if !IsSuccessfulRepair(metrics) {
		t.Fatalf("expected successful repair: %+v", metrics)
	}
	if !metrics.FullyRepaired {
		t.Fatal("expected fully repaired")
	}
}
