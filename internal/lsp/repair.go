package lsp

import "time"

type DiagnosticSnapshot struct {
	Timestamp     time.Time
	Diagnostics   map[string][]Diagnostic
	TotalErrors   int
	TotalWarnings int
}

type DiagnosticsDelta struct {
	ErrorsFixed    int
	ErrorsAdded    int
	WarningsFixed  int
	WarningsAdded  int
	NetImprovement int
}

type RepairMetrics struct {
	ErrorReductionRate  float64
	FullyRepaired       bool
	NewIssuesIntroduced int
	QualityScore        float64
	Attempts            int
	RepairTime          time.Duration
	EfficiencyScore     float64
}

func (c *Client) Snapshot() *DiagnosticSnapshot {
	snapshot := &DiagnosticSnapshot{
		Timestamp:   time.Now(),
		Diagnostics: make(map[string][]Diagnostic),
	}
	c.diagnostics.Range(func(uri string, diags []Diagnostic) bool {
		copied := append([]Diagnostic(nil), diags...)
		snapshot.Diagnostics[uri] = copied
		for _, diag := range copied {
			switch diag.Severity {
			case SeverityWarning:
				snapshot.TotalWarnings++
			case SeverityInformation, SeverityHint:
			default:
				snapshot.TotalErrors++
			}
		}
		return true
	})
	return snapshot
}

func CalculateDelta(before, after *DiagnosticSnapshot) DiagnosticsDelta {
	if before == nil {
		before = &DiagnosticSnapshot{}
	}
	if after == nil {
		after = &DiagnosticSnapshot{}
	}

	delta := DiagnosticsDelta{}
	if before.TotalErrors >= after.TotalErrors {
		delta.ErrorsFixed = before.TotalErrors - after.TotalErrors
	} else {
		delta.ErrorsAdded = after.TotalErrors - before.TotalErrors
	}
	if before.TotalWarnings >= after.TotalWarnings {
		delta.WarningsFixed = before.TotalWarnings - after.TotalWarnings
	} else {
		delta.WarningsAdded = after.TotalWarnings - before.TotalWarnings
	}
	delta.NetImprovement = 2*(delta.ErrorsFixed-delta.ErrorsAdded) + (delta.WarningsFixed - delta.WarningsAdded)
	return delta
}

func CalculateRepairMetrics(before, after *DiagnosticSnapshot, attempts int, duration time.Duration) *RepairMetrics {
	delta := CalculateDelta(before, after)
	metrics := &RepairMetrics{Attempts: attempts, RepairTime: duration}
	if before == nil || before.TotalErrors == 0 {
		metrics.ErrorReductionRate = 1
	} else {
		metrics.ErrorReductionRate = float64(delta.ErrorsFixed) / float64(before.TotalErrors)
	}
	if after == nil || after.TotalErrors == 0 {
		metrics.FullyRepaired = true
	}
	metrics.NewIssuesIntroduced = delta.ErrorsAdded + delta.WarningsAdded
	metrics.QualityScore = 100 - float64(metrics.NewIssuesIntroduced*20)
	if metrics.QualityScore < 0 {
		metrics.QualityScore = 0
	}
	switch attempts {
	case 0, 1:
		metrics.EfficiencyScore = 100
	case 2:
		metrics.EfficiencyScore = 80
	case 3:
		metrics.EfficiencyScore = 60
	default:
		metrics.EfficiencyScore = 40
	}
	return metrics
}

func IsSuccessfulRepair(metrics *RepairMetrics) bool {
	return metrics != nil && metrics.ErrorReductionRate > 0.5 && metrics.NewIssuesIntroduced == 0 && metrics.QualityScore >= 80
}
