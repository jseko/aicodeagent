package tools

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AuditLog 单次操作审计记录
type AuditLog struct {
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id"`
	ToolName  string                 `json:"tool_name"`
	Action    string                 `json:"action"`
	Params    map[string]interface{} `json:"params"`
	Result    string                 `json:"result"`
	Reason    string                 `json:"reason,omitempty"`
}

// AuditLogWriter 审计日志写入接口（由调用方实现持久化）
type AuditLogWriter interface {
	WriteLog(log AuditLog) error
}

// AsyncAuditLogger 异步审计日志器（缓冲通道 + 批量提交）
type AsyncAuditLogger struct {
	buffer   chan AuditLog
	writer   AuditLogWriter
	wg       sync.WaitGroup
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewAsyncAuditLogger 创建异步审计日志器
func NewAsyncAuditLogger(writer AuditLogWriter, bufferSize int) *AsyncAuditLogger {
	if bufferSize <= 0 {
		bufferSize = 256
	}
	l := &AsyncAuditLogger{
		buffer: make(chan AuditLog, bufferSize),
		writer: writer,
		stopCh: make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	return l
}

// LogOperation 记录成功操作（异步，不阻塞调用方）
func (l *AsyncAuditLogger) LogOperation(sessionID, toolName, action string, params map[string]interface{}) {
	select {
	case l.buffer <- AuditLog{
		Timestamp: time.Now(),
		SessionID: sessionID,
		ToolName:  toolName,
		Action:    action,
		Params:    sanitizeAuditParams(params),
		Result:    "success",
	}:
	default:
		// 缓冲区满时丢弃（不阻塞业务操作）
	}
}

// LogViolation 记录违规操作
func (l *AsyncAuditLogger) LogViolation(layer string, sessionID, toolName, action string, params map[string]interface{}, err error) {
	select {
	case l.buffer <- AuditLog{
		Timestamp: time.Now(),
		SessionID: sessionID,
		ToolName:  toolName,
		Action:    action,
		Params:    sanitizeAuditParams(params),
		Result:    "denied",
		Reason:    fmt.Sprintf("[%s] %s", layer, err.Error()),
	}:
	default:
	}
}

const auditMaskedValue = "***MASKED***"

func sanitizeAuditParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	cleaned := make(map[string]interface{}, len(params))
	for key, value := range params {
		if secretKeyPattern.MatchString(key) {
			cleaned[key] = auditMaskedValue
			continue
		}
		cleaned[key] = sanitizeAuditValue(value)
	}
	return cleaned
}

func sanitizeAuditValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return sanitizeAuditParams(v)
	case []interface{}:
		cleaned := make([]interface{}, len(v))
		for i, item := range v {
			cleaned[i] = sanitizeAuditValue(item)
		}
		return cleaned
	case string:
		for _, pattern := range secretValuePatterns {
			if pattern.MatchString(v) {
				return auditMaskedValue
			}
		}
	}
	return value
}

// flushLoop 批量提交循环（每 100 条或 5 秒）
func (l *AsyncAuditLogger) flushLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var batch []AuditLog
	for {
		select {
		case <-l.stopCh:
			if len(batch) > 0 {
				l.flush(batch)
			}
			// 排空缓冲区
			for {
				select {
				case entry := <-l.buffer:
					l.flushOne(entry)
				default:
					return
				}
			}
		case entry := <-l.buffer:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				l.flush(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.flush(batch)
				batch = nil
			}
		}
	}
}

func (l *AsyncAuditLogger) flush(batch []AuditLog) {
	for _, entry := range batch {
		l.flushOne(entry)
	}
}

func (l *AsyncAuditLogger) flushOne(entry AuditLog) {
	if l.writer != nil {
		_ = l.writer.WriteLog(entry)
	}
}

// Stop 停止日志器（等待所有缓冲日志写入完成）
func (l *AsyncAuditLogger) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
		l.wg.Wait()
	})
}

// LogSuccess 兼容旧 AuditLogger 接口
func (l *AsyncAuditLogger) LogSuccess(toolName string, params json.RawMessage) {
	l.LogOperation("", toolName, "execute", nil)
}

// LogRejection 兼容旧 AuditLogger 接口
func (l *AsyncAuditLogger) LogRejection(toolName string, params json.RawMessage, reason string) {
	l.LogViolation("tool", "", toolName, "execute", nil, fmt.Errorf("%s", reason))
}
