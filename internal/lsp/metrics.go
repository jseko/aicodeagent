package lsp

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

type RepairMetricsCollector struct {
	mu       sync.RWMutex
	sessions map[string]*RepairSession
}

type RepairSession struct {
	SessionID  string
	StartTime  time.Time
	EndTime    time.Time
	Attempts   int
	Success    bool
	ErrorTypes []string
	RepairTime time.Duration
	RolledBack bool
}

type ErrorTypeStat struct {
	Success int
	Total   int
}

type MetricsReport struct {
	TotalSessions     int
	SuccessRate       float64
	AverageAttempts   float64
	AverageRepairTime time.Duration
	RollbackRate      float64
	ErrorTypeStats    map[string]ErrorTypeStat
}

func NewRepairMetricsCollector() *RepairMetricsCollector {
	return &RepairMetricsCollector{sessions: make(map[string]*RepairSession)}
}

func (c *RepairMetricsCollector) Record(session *RepairSession) {
	if session == nil || session.SessionID == "" {
		return
	}
	if session.RepairTime == 0 && !session.StartTime.IsZero() && !session.EndTime.IsZero() {
		session.RepairTime = session.EndTime.Sub(session.StartTime)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copySession := *session
	copySession.ErrorTypes = append([]string(nil), session.ErrorTypes...)
	c.sessions[session.SessionID] = &copySession
}

func (c *RepairMetricsCollector) CollectMetrics() *MetricsReport {
	c.mu.RLock()
	defer c.mu.RUnlock()

	report := &MetricsReport{TotalSessions: len(c.sessions), ErrorTypeStats: make(map[string]ErrorTypeStat)}
	if report.TotalSessions == 0 {
		return report
	}

	var totalAttempts int
	var totalTime time.Duration
	var successCount int
	var rollbackCount int
	for _, session := range c.sessions {
		totalAttempts += session.Attempts
		totalTime += session.RepairTime
		if session.Success {
			successCount++
		}
		if session.RolledBack {
			rollbackCount++
		}
		for _, errorType := range session.ErrorTypes {
			stat := report.ErrorTypeStats[errorType]
			stat.Total++
			if session.Success {
				stat.Success++
			}
			report.ErrorTypeStats[errorType] = stat
		}
	}

	report.SuccessRate = float64(successCount) / float64(report.TotalSessions)
	report.AverageAttempts = float64(totalAttempts) / float64(report.TotalSessions)
	report.AverageRepairTime = totalTime / time.Duration(report.TotalSessions)
	report.RollbackRate = float64(rollbackCount) / float64(report.TotalSessions)
	return report
}

func (r *MetricsReport) PrintReport(w io.Writer) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, "==================================================")
	fmt.Fprintln(w, "LSP Repair Metrics")
	fmt.Fprintln(w, "==================================================")
	fmt.Fprintf(w, "Total Sessions: %d\n", r.TotalSessions)
	fmt.Fprintf(w, "Success Rate: %.2f%%\n", r.SuccessRate*100)
	fmt.Fprintf(w, "Average Attempts: %.2f\n", r.AverageAttempts)
	fmt.Fprintf(w, "Average Repair Time: %v\n", r.AverageRepairTime)
	fmt.Fprintf(w, "Rollback Rate: %.2f%%\n", r.RollbackRate*100)

	keys := make([]string, 0, len(r.ErrorTypeStats))
	for key := range r.ErrorTypeStats {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintln(w, "Error Types:")
	for _, key := range keys {
		stat := r.ErrorTypeStats[key]
		rate := 0.0
		if stat.Total > 0 {
			rate = float64(stat.Success) / float64(stat.Total) * 100
		}
		fmt.Fprintf(w, "  %s: %.2f%% (%d/%d)\n", key, rate, stat.Success, stat.Total)
	}
}
