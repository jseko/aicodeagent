// Package testutil 测试基础设施（例9-16）
package testutil

import (
	"os"
	"testing"
	"time"

	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
)

// TestFixtures 测试夹具：提供测试所需的共享资源
type TestFixtures struct {
	TempDir string
	Broker  *pubsub.Broker[events.Event]
}

// NewTestFixtures 创建测试夹具
func NewTestFixtures(t *testing.T) *TestFixtures {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "aicode-tui-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	broker := pubsub.NewBroker[events.Event]()

	return &TestFixtures{
		TempDir: tempDir,
		Broker:  broker,
	}
}

// Cleanup 清理测试资源
func (f *TestFixtures) Cleanup(t *testing.T) {
	t.Helper()
	f.Broker.Shutdown()
	os.RemoveAll(f.TempDir)
}

// TestSession 测试用会话数据结构
type TestSession struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

// WithSession 会话生命周期管理辅助（例9-16）
func WithSession(t *testing.T, fixtures *TestFixtures, name string, fn func(*TestSession)) {
	t.Helper()

	session := &TestSession{
		ID:        "test-session-" + name,
		Title:     name,
		CreatedAt: time.Now(),
	}

	defer func() {
		// 清理与会话相关的资源
		fixtures.Broker.Shutdown()
		fixtures.Broker = pubsub.NewBroker[events.Event]()
	}()

	fn(session)
}
