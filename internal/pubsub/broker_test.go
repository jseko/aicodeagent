package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestNewBroker(t *testing.T) {
	b := NewBroker[string]()
	if b == nil {
		t.Fatal("NewBroker returned nil")
	}
	if b.channelBufferSize != defaultBufferSize {
		t.Errorf("expected buffer size %d, got %d", defaultBufferSize, b.channelBufferSize)
	}
	if b.GetSubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers, got %d", b.GetSubscriberCount())
	}
}

func TestSubscribe(t *testing.T) {
	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	if b.GetSubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", b.GetSubscriberCount())
	}
}

func TestPublish(t *testing.T) {
	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	b.Publish("hello")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Errorf("expected 'hello', got '%s'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for message")
	}
}

func TestPublishMustDeliver(t *testing.T) {
	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	b.PublishMustDeliver(context.Background(), "must_deliver")

	select {
	case msg := <-ch:
		if msg != "must_deliver" {
			t.Errorf("expected 'must_deliver', got '%s'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for message")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := NewBroker[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := b.Subscribe(ctx)
	ch2 := b.Subscribe(ctx)
	ch3 := b.Subscribe(ctx)

	if b.GetSubscriberCount() != 3 {
		t.Errorf("expected 3 subscribers, got %d", b.GetSubscriberCount())
	}

	b.Publish(42)

	for i, ch := range []<-chan int{ch1, ch2, ch3} {
		select {
		case msg := <-ch:
			if msg != 42 {
				t.Errorf("subscriber %d: expected 42, got %d", i, msg)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestShutdown(t *testing.T) {
	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)
	b.Shutdown()

	// Channel should be closed after shutdown
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after shutdown")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}

	if b.GetSubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after shutdown, got %d", b.GetSubscriberCount())
	}
}

func TestSubscribeAfterShutdown(t *testing.T) {
	b := NewBroker[string]()
	b.Shutdown()

	ctx := context.Background()
	ch := b.Subscribe(ctx)

	// Channel should be closed immediately
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed when subscribing after shutdown")
		}
	default:
		t.Error("channel should be closed immediately after subscribe on shutdown broker")
	}
}

func TestShutdownIdempotent(t *testing.T) {
	b := NewBroker[string]()
	b.Shutdown()
	// Second shutdown should not panic
	b.Shutdown()
}

func TestContextCancelCleanup(t *testing.T) {
	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())

	ch := b.Subscribe(ctx)
	if b.GetSubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", b.GetSubscriberCount())
	}

	// Cancel context, goroutine needs time to clean up
	cancel()
	time.Sleep(50 * time.Millisecond)

	if b.GetSubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", b.GetSubscriberCount())
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestDropCount(t *testing.T) {
	b := NewBrokerWithOptions[int](1) // Small buffer to force drops
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Fill the buffer
	b.Publish(1)
	// This should drop because buffer is full and nobody is reading
	b.Publish(2)
	b.Publish(3)
	b.Publish(4)

	if b.DropCount() == 0 {
		t.Error("expected some drops with full buffer")
	}

	// Drain to avoid goroutine leak
	<-ch
}

func TestMustDeliverDropCount(t *testing.T) {
	b := NewBrokerWithOptions[int](1)
	b.SetMustDeliverTimeout(1 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Fill the buffer first
	b.Publish(1)

	// Now buffer is full, PublishMustDeliver should timeout
	b.PublishMustDeliver(context.Background(), 42)

	if b.MustDeliverDropCount() == 0 {
		t.Error("expected must-deliver drop with full buffer and short timeout")
	}

	// Drain
	<-ch
}

func TestPublishConcurrency(t *testing.T) {
	b := NewBroker[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			b.Publish(v)
		}(i)
	}
	wg.Wait()

	// Drain some messages (should not panic or deadlock)
	count := 0
	timeout := time.After(200 * time.Millisecond)
drain:
	for count < 100 {
		select {
		case <-ch:
			count++
		case <-timeout:
			break drain
		}
	}
	// At minimum, some messages should arrive
	if count == 0 {
		t.Error("no messages received after concurrent publishes")
	}
}

func TestSlowConsumer(t *testing.T) {
	b := NewBrokerWithOptions[int](4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Publish faster than consumer reads
	for i := 0; i < 20; i++ {
		b.Publish(i)
	}

	// Consumer reads slowly
	count := 0
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case <-ch:
			count++
		case <-timeout:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("slow consumer should receive at least some messages")
	}
	// With buffer size 4 and 20 publishes, some should have been dropped
	if b.DropCount() == 0 {
		t.Error("expected drops with slow consumer and small buffer")
	}
}

func TestGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	b := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())

	ch := b.Subscribe(ctx)
	b.Publish("test")

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}
