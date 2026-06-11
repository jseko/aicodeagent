package tools

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type testAuditWriter struct {
	mu   sync.Mutex
	logs []AuditLog
}

func (w *testAuditWriter) WriteLog(log AuditLog) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logs = append(w.logs, log)
	return nil
}

func (w *testAuditWriter) countByResult(result string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, l := range w.logs {
		if l.Result == result {
			n++
		}
	}
	return n
}

func TestAsyncAuditLoggerLogOperation(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 64)
	defer logger.Stop()

	logger.LogOperation("s1", "view", "read", map[string]interface{}{"file": "a.go"})
	logger.LogOperation("s1", "bash", "execute", map[string]interface{}{"command": "ls"})

	// 停止以排空缓冲
	logger.Stop()

	if n := writer.countByResult("success"); n != 2 {
		t.Fatalf("expected 2 success logs, got %d", n)
	}
}

func TestAsyncAuditLoggerMasksSensitiveParams(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 64)
	defer logger.Stop()

	logger.LogOperation("s1", "write_file", "write", map[string]interface{}{
		"content": "token := \"abcdef1234567890abcdef\"",
		"config": map[string]interface{}{
			"authorization": "Bearer abcdef1234567890abcdef",
		},
	})
	logger.Stop()

	if len(writer.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(writer.logs))
	}
	if writer.logs[0].Params["content"] != auditMaskedValue {
		t.Fatalf("expected content to be masked, got %#v", writer.logs[0].Params["content"])
	}
	config, ok := writer.logs[0].Params["config"].(map[string]interface{})
	if !ok || config["authorization"] != auditMaskedValue {
		t.Fatalf("expected nested authorization to be masked, got %#v", writer.logs[0].Params["config"])
	}
}

func TestAsyncAuditLoggerLogViolation(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 64)
	defer logger.Stop()

	logger.LogViolation("bash_filter", "s1", "bash", "execute",
		map[string]interface{}{"command": "rm -rf /"},
		fmt.Errorf("命令被禁止"))

	logger.Stop()

	if n := writer.countByResult("denied"); n != 1 {
		t.Fatalf("expected 1 denied log, got %d", n)
	}
}

func TestAsyncAuditLoggerBufferFullDoesNotBlock(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 2) // 极小缓冲区
	defer logger.Stop()

	// 填满缓冲区后再写入不应阻塞
	for i := 0; i < 10; i++ {
		logger.LogOperation("s1", "bash", "run", map[string]interface{}{"i": i})
	}

	logger.Stop()
	// 至少应有部分日志被写入（缓冲区满时丢弃部分）
	if writer.countByResult("success") == 0 {
		t.Fatal("expected at least some logs to be written")
	}
}

func TestAsyncAuditLoggerStopFlushesRemaining(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 256)

	logger.LogOperation("s1", "view", "read", nil)
	logger.Stop()

	if n := writer.countByResult("success"); n != 1 {
		t.Fatalf("expected 1 log after stop, got %d", n)
	}
}

func TestAsyncAuditLoggerImplementsAuditLoggerInterface(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 64)
	defer logger.Stop()

	// 验证 AsyncAuditLogger 实现 AuditLogger 接口
	var _ AuditLogger = logger

	logger.LogSuccess("view", jsonRaw(`{"file_path":"test.go"}`))
	logger.LogRejection("bash", jsonRaw(`{"command":"sudo"}`), "禁止")
	logger.Stop()

	if n := writer.countByResult("success"); n != 1 {
		t.Fatalf("expected 1 success via interface, got %d", n)
	}
	if n := writer.countByResult("denied"); n != 1 {
		t.Fatalf("expected 1 denied via interface, got %d", n)
	}
}

func TestAsyncAuditLoggerBatchFlush(t *testing.T) {
	writer := &testAuditWriter{}
	logger := NewAsyncAuditLogger(writer, 256)
	// 写入 150 条日志，应触发至少 1 次批量提交（阈值 100）
	for i := 0; i < 150; i++ {
		logger.LogOperation("s1", "bash", "run", map[string]interface{}{"i": i})
	}
	time.Sleep(100 * time.Millisecond) // 给 flush 时间
	logger.Stop()

	n := writer.countByResult("success")
	if n < 100 {
		t.Fatalf("expected at least 100 logs flushed, got %d", n)
	}
}

func TestAuditLogStructFields(t *testing.T) {
	log := AuditLog{
		Timestamp: time.Now(),
		SessionID: "s1",
		ToolName:  "bash",
		Action:    "execute",
		Params:    map[string]interface{}{"command": "ls"},
		Result:    "success",
	}

	if log.ToolName != "bash" || log.Result != "success" || log.SessionID != "s1" {
		t.Fatal("AuditLog fields mismatch")
	}
}
