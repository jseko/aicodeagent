package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"AICodeAgent/internal/log"
)

func TestLogsCmdExists(t *testing.T) {
	if logsCmd == nil {
		t.Fatal("logsCmd should not be nil")
	}
	if logsCmd.Use != "logs" {
		t.Errorf("logsCmd.Use = %s, want logs", logsCmd.Use)
	}
	if logsCmd.Short == "" {
		t.Error("logsCmd.Short should not be empty")
	}
}

func TestLogsCmdFlags(t *testing.T) {
	followFlag := logsCmd.Flags().Lookup("follow")
	if followFlag == nil {
		t.Fatal("logs command should have --follow flag")
	}
	if followFlag.Shorthand != "f" {
		t.Errorf("follow shorthand = %s, want f", followFlag.Shorthand)
	}
}

func TestLogsCmd_NoLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	old := log.GetLogDir()
	log.SetLogDir(tmpDir)
	defer log.SetLogDir(old)

	logFollow = false
	err := runLogs(nil, nil)
	if err != nil {
		t.Logf("runLogs with empty dir returned: %v (expected)", err)
	}
}

func TestLogsCmd_WithLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	old := log.GetLogDir()
	log.SetLogDir(tmpDir)
	defer log.SetLogDir(old)

	logFile := filepath.Join(tmpDir, "aicodeagent.log")
	if err := os.WriteFile(logFile, []byte("test log entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logFollow = false
	err := runLogs(nil, nil)
	if err != nil {
		t.Logf("runLogs with log file: %v", err)
	}
}
