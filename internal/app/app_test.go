package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"

	"go.uber.org/goleak"
)

// subscriberFixture 订阅者测试夹具，封装测试环境
type subscriberFixture struct {
	broker   *pubsub.Broker[events.Event]
	ctx      context.Context
	cancel   context.CancelFunc
	ch       <-chan events.Event
	received []events.Event
	mu       sync.Mutex
	wg       sync.WaitGroup
}

// newSubscriberFixture 创建测试夹具并注册清理函数
func newSubscriberFixture(t *testing.T) *subscriberFixture {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	broker := pubsub.NewBroker[events.Event]()
	ch := broker.Subscribe(ctx)

	f := &subscriberFixture{
		broker: broker,
		ctx:    ctx,
		cancel: cancel,
		ch:     ch,
	}

	// 启动后台收集器
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				f.mu.Lock()
				f.received = append(f.received, evt)
				f.mu.Unlock()
			}
		}
	}()

	t.Cleanup(func() {
		f.cancel()
		f.wg.Wait()
	})

	return f
}

func (f *subscriberFixture) receivedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.received)
}

// TestSetupSubscriber_NormalFlow 验证正常流程：发布事件 → 订阅者接收
func TestSetupSubscriber_NormalFlow(t *testing.T) {
	f := newSubscriberFixture(t)

	// 发布 5 个流式 chunk + 1 个 IsDone 事件
	now := time.Now()
	for i := 0; i < 5; i++ {
		f.broker.PublishMustDeliver(f.ctx,
			events.AgentThink{SessionID: "s1", Content: "chunk", Time: now})
	}
	f.broker.PublishMustDeliver(f.ctx,
		events.AgentThink{SessionID: "s1", IsDone: true, Time: now})

	time.Sleep(100 * time.Millisecond)

	count := f.receivedCount()
	if count < 5 {
		t.Errorf("expected at least 5 events, got %d", count)
	}

	// 验证 IsDone 存在
	f.mu.Lock()
	hasIsDone := false
	for _, evt := range f.received {
		if think, ok := evt.(events.AgentThink); ok && think.IsDone {
			hasIsDone = true
			break
		}
	}
	f.mu.Unlock()

	if !hasIsDone {
		t.Error("expected AgentThink with IsDone=true")
	}
}

// TestSetupSubscriber_SlowConsumer 验证慢消费者消息丢弃行为
func TestSetupSubscriber_SlowConsumer(t *testing.T) {
	// 使用小缓冲区强制丢弃
	broker := pubsub.NewBrokerWithOptions[events.Event](2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch := broker.Subscribe(ctx)

	// 快速发布，缓冲区满后丢弃
	now := time.Now()
	for i := 0; i < 50; i++ {
		broker.Publish(events.AgentThink{SessionID: "s1", Content: "data", Time: now})
	}

	// 慢速消费
	count := 0
	timeout := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case <-ch:
			count++
		case <-timeout:
			break drain
		}
	}

	if count == 0 {
		t.Error("slow consumer should receive at least some messages")
	}
	if broker.DropCount() == 0 {
		t.Error("expected drops with small buffer and fast publisher")
	}
}

// TestSetupSubscriber_NoTimerLeak 验证测试结束后无 goroutine 泄漏
func TestSetupSubscriber_NoTimerLeak(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	ch := broker.Subscribe(ctx)

	// 收集 goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
			}
		}
	}()

	now := time.Now()
	broker.PublishMustDeliver(ctx,
		events.UserMessage{SessionID: "s1", Content: "hello", Time: now})
	broker.PublishMustDeliver(ctx,
		events.AgentThink{SessionID: "s1", IsDone: true, Time: now})

	time.Sleep(100 * time.Millisecond)

	// 先手动清理，再检查 goroutine 泄漏
	cancel()
	wg.Wait()
	broker.Shutdown()

	goleak.VerifyNone(t)
}

// TestEventPipeline 端到端集成测试：模拟 UserMessage → Coordinator → TUI 完整事件流
func TestEventPipeline(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 模拟 TUI 订阅
	tuiCh := broker.Subscribe(ctx)

	// 模拟 Coordinator 订阅
	coordCh := broker.Subscribe(ctx)

	// 收集 TUI 收到的事件
	var tuiEvents []events.Event
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-tuiCh:
				if !ok {
					return
				}
				mu.Lock()
				tuiEvents = append(tuiEvents, evt)
				mu.Unlock()
			}
		}
	}()

	// 模拟 Coordinator：接收 UserMessage，发布流式 AgentThink，最后发布 IsDone
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-coordCh:
				if !ok {
					return
				}
				msg, ok := evt.(events.UserMessage)
				if !ok {
					continue
				}
				// 模拟流式 LLM 响应：发布多个 chunk
				chunks := []string{"Hello", " World", " from", " AI"}
				for _, chunk := range chunks {
					broker.Publish(events.AgentThink{
						SessionID: msg.SessionID,
						Content:   chunk,
						Time:      time.Now(),
					})
				}
				// 终端事件
				broker.PublishMustDeliver(ctx,
					events.AgentThink{SessionID: msg.SessionID, IsDone: true, Time: time.Now()})
				return
			}
		}
	}()

	// 用户发送消息（模拟 TUI 输入）
	broker.Publish(events.UserMessage{
		SessionID: "test-session",
		Content:   "Hello AI",
		Time:      time.Now(),
	})

	// 等待 Coordinator 完成处理
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	received := len(tuiEvents)
	mu.Unlock()

	if received < 4 {
		t.Errorf("expected at least 4 events (3 chunks + IsDone), got %d", received)
	}

	// 验证 IsDone 事件存在
	hasIsDone := false
	mu.Lock()
	for _, evt := range tuiEvents {
		if think, ok := evt.(events.AgentThink); ok && think.IsDone {
			hasIsDone = true
			break
		}
	}
	mu.Unlock()

	if !hasIsDone {
		t.Error("expected AgentThink with IsDone=true in TUI events")
	}
}

// TestStreamAccumulation 模拟 TUI 流式内容追加渲染
func TestStreamAccumulation(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := broker.Subscribe(ctx)

	// 发布模拟流式 chunk
	go func() {
		for _, content := range []string{"package ", "main\n\n", "import ", "\"fmt\"\n\n", "func main()", " {\n", "    fmt.Println(\"hello\")\n", "}"} {
			broker.Publish(events.AgentThink{
				SessionID: "s1",
				Content:   content,
				Time:      time.Now(),
			})
		}
		broker.PublishMustDeliver(ctx,
			events.AgentThink{SessionID: "s1", IsDone: true, Time: time.Now()})
	}()

	// 模拟 TUI 的 appendStreamContent 逻辑
	var streamContent string
	done := false
	for !done {
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for events")
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if think, ok := evt.(events.AgentThink); ok {
				if think.IsDone {
					done = true
					break
				}
				streamContent += think.Content
			}
		}
	}

	expected := "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"hello\")\n}"
	if streamContent != expected {
		t.Errorf("stream accumulation mismatch:\nexpected: %s\ngot:      %s", expected, streamContent)
	}
}

// TestErrorPropagation 测试错误事件传播
func TestErrorPropagation(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := broker.Subscribe(ctx)

	// 模拟 LLM 调用失败
	broker.PublishMustDeliver(ctx,
		events.ErrorEvent{SessionID: "s1", Error: "LLM API timeout", Time: time.Now()})

	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for error event")
	case evt := <-ch:
		errEvt, ok := evt.(events.ErrorEvent)
		if !ok {
			t.Fatalf("expected ErrorEvent, got %T", evt)
		}
		if errEvt.Error != "LLM API timeout" {
			t.Errorf("error message mismatch: %s", errEvt.Error)
		}
	}
}

// TestMultipleSubscribersAndEventTypes 多订阅者接收不同事件类型
func TestMultipleSubscribersAndEventTypes(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 多个订阅者
	sub1 := broker.Subscribe(ctx)
	sub2 := broker.Subscribe(ctx)
	sub3 := broker.Subscribe(ctx)

	// 发布不同类型的事件
	eventsToPublish := []events.Event{
		events.UserMessage{SessionID: "s1", Content: "hello", Time: time.Now()},
		events.AgentThink{SessionID: "s1", Content: "chunk1", Time: time.Now()},
		events.ToolCall{SessionID: "s1", ToolName: "read_file", Time: time.Now()},
		events.ToolResult{SessionID: "s1", ToolName: "read_file", Result: "ok", Time: time.Now()},
		events.AgentThink{SessionID: "s1", IsDone: true, Time: time.Now()},
	}

	for _, evt := range eventsToPublish {
		broker.PublishMustDeliver(ctx, evt)
	}

	time.Sleep(100 * time.Millisecond)

	// 验证每个订阅者都收到了相同数量的事件
	subs := []<-chan events.Event{sub1, sub2, sub3}
	for i, sub := range subs {
		count := 0
		drain := true
		for drain {
			select {
			case <-sub:
				count++
			default:
				drain = false
			}
		}
		if count != len(eventsToPublish) {
			t.Errorf("subscriber %d: expected %d events, got %d", i+1, len(eventsToPublish), count)
		}
	}
}

// TestPublishMustDeliverGuarantee 验证 PublishMustDeliver 的有界阻塞保证
func TestPublishMustDeliverGuarantee(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := broker.Subscribe(ctx)

	// PublishMustDeliver 应该在有缓冲区空间时立即可达
	for i := 0; i < 100; i++ {
		broker.PublishMustDeliver(ctx,
			events.AgentThink{SessionID: "s1", Content: "msg", Time: time.Now()})
	}

	// 清点消息数
	count := 0
	timeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case <-ch:
			count++
		case <-timeout:
			break drain
		}
	}

	if count != 100 {
		t.Errorf("PublishMustDeliver: expected 100 events delivered, got %d", count)
	}
	if broker.MustDeliverDropCount() > 0 {
		t.Errorf("PublishMustDeliver: unexpected drops with adequate buffer, dropCount=%d",
			broker.MustDeliverDropCount())
	}
}

// TestContextCancellationCleanup 测试 Context 取消后 goroutine 清理
func TestContextCancellationCleanup(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx, cancel := context.WithCancel(context.Background())

	ch := broker.Subscribe(ctx)
	if broker.GetSubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", broker.GetSubscriberCount())
	}

	// 取消 Context，触发 goroutine 清理
	cancel()
	time.Sleep(100 * time.Millisecond)

	if broker.GetSubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", broker.GetSubscriberCount())
	}

	// Channel 应已关闭
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancel")
		}
	default:
		t.Error("channel should be closed (not just empty)")
	}
}

// TestGracefulShutdown 优雅关闭：无 goroutine 泄漏
func TestGracefulShutdown(t *testing.T) {
	broker := pubsub.NewBroker[events.Event]()
	ctx := context.Background()

	// 创建多个订阅者
	for i := 0; i < 50; i++ {
		broker.Subscribe(ctx)
	}

	if broker.GetSubscriberCount() != 50 {
		t.Fatalf("expected 50 subscribers, got %d", broker.GetSubscriberCount())
	}

	// 发布一些事件
	for i := 0; i < 10; i++ {
		broker.Publish(events.AgentThink{
			SessionID: "s1",
			Content:   "data",
			Time:      time.Now(),
		})
	}

	// 优雅关闭
	broker.Shutdown()

	if broker.GetSubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after shutdown, got %d", broker.GetSubscriberCount())
	}

	// 关闭后再订阅，应返回已关闭 channel
	ch := broker.Subscribe(ctx)
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("subscribe after shutdown should return closed channel")
		}
	default:
		t.Error("subscribe after shutdown should return immediately closed channel")
	}
}
