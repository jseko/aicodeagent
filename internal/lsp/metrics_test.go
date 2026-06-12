package lsp

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRepairMetricsCollector(t *testing.T) {
	collector := NewRepairMetricsCollector()
	collector.Record(&RepairSession{
		SessionID:  "s1",
		Attempts:   2,
		Success:    true,
		ErrorTypes: []string{string(ErrorTypeSyntax), string(ErrorTypeUndefined)},
		RepairTime: 2 * time.Second,
	})
	collector.Record(&RepairSession{
		SessionID:  "s2",
		Attempts:   3,
		Success:    false,
		ErrorTypes: []string{string(ErrorTypeUndefined)},
		RepairTime: 4 * time.Second,
		RolledBack: true,
	})

	report := collector.CollectMetrics()
	if report.TotalSessions != 2 {
		t.Fatalf("TotalSessions = %d", report.TotalSessions)
	}
	if report.SuccessRate != 0.5 {
		t.Fatalf("SuccessRate = %f", report.SuccessRate)
	}
	if report.AverageAttempts != 2.5 {
		t.Fatalf("AverageAttempts = %f", report.AverageAttempts)
	}
	if report.ErrorTypeStats[string(ErrorTypeUndefined)].Total != 2 {
		t.Fatalf("undefined total = %+v", report.ErrorTypeStats[string(ErrorTypeUndefined)])
	}
}

func TestMetricsReportPrintReport(t *testing.T) {
	report := &MetricsReport{
		TotalSessions: 1,
		SuccessRate:   1,
		ErrorTypeStats: map[string]ErrorTypeStat{
			string(ErrorTypeSyntax): {Success: 1, Total: 1},
		},
	}
	var buf bytes.Buffer
	report.PrintReport(&buf)
	output := buf.String()
	if !strings.Contains(output, "LSP Repair Metrics") || !strings.Contains(output, "syntax") {
		t.Fatalf("unexpected report: %s", output)
	}
}
