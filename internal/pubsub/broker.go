// Package pubsub provides a lightweight in-process broker for fan-out
// event delivery between services and the UI.
//
// Delivery semantics:
//   - Publish is best-effort and lossy under contention. If a subscriber's
//     channel is full, the event is dropped, a warning is logged, and
//     dropCount is incremented. Use for high-frequency intermediate updates
//     (e.g. streaming token deltas).
//   - PublishMustDeliver is bounded-blocking. For each subscriber it first
//     tries a non-blocking send, then falls back to a blocking send with a
//     hard timeout (default 50ms). On timeout the event is dropped,
//     mustDeliverDropCount is incremented. Use for terminal events
//     (finish, error, tool result) that must not be silently lost.
package pubsub

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBufferSize        = 256
	defaultMustDeliverTimeout = 50 * time.Millisecond
)

// Broker 泛型事件总线，T为事件类型
type Broker[T any] struct {
	subs                 map[chan T]struct{}
	mu                   sync.RWMutex
	done                 chan struct{}
	subCount             int
	channelBufferSize    int
	mustDeliverTimeout   time.Duration
	dropCount            atomic.Uint64
	mustDeliverDropCount atomic.Uint64
}

// Publisher 定义发布行为接口
type Publisher[T any] interface {
	Publish(event T)
	PublishMustDeliver(ctx context.Context, event T)
}

// Subscriber 定义订阅行为接口
type Subscriber[T any] interface {
	Subscribe(ctx context.Context) <-chan T
}

func NewBroker[T any]() *Broker[T] {
	return NewBrokerWithOptions[T](defaultBufferSize)
}

func NewBrokerWithOptions[T any](channelBufferSize int) *Broker[T] {
	return &Broker[T]{
		subs:               make(map[chan T]struct{}),
		done:               make(chan struct{}),
		channelBufferSize:  channelBufferSize,
		mustDeliverTimeout: defaultMustDeliverTimeout,
	}
}

// SetMustDeliverTimeout overrides the per-subscriber timeout used by
// PublishMustDeliver. Intended primarily for tests.
func (b *Broker[T]) SetMustDeliverTimeout(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if d <= 0 {
		b.mustDeliverTimeout = defaultMustDeliverTimeout
		return
	}
	b.mustDeliverTimeout = d
}

func (b *Broker[T]) Subscribe(ctx context.Context) <-chan T {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 已 Shutdown，返回已关闭 channel
	select {
	case <-b.done:
		ch := make(chan T)
		close(ch)
		return ch
	default:
	}

	sub := make(chan T, b.channelBufferSize)
	b.subs[sub] = struct{}{}
	b.subCount++

	go func() {
		<-ctx.Done()

		b.mu.Lock()
		defer b.mu.Unlock()

		// 二次检查：避免 Shutdown 已清理后重复 close
		select {
		case <-b.done:
			return
		default:
		}

		delete(b.subs, sub)
		close(sub)
		b.subCount--
	}()

	return sub
}

func (b *Broker[T]) GetSubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.subCount
}

// DropCount returns the cumulative number of events dropped by Publish.
func (b *Broker[T]) DropCount() uint64 {
	return b.dropCount.Load()
}

// MustDeliverDropCount returns the cumulative number of events dropped
// by PublishMustDeliver after the per-subscriber timeout expired.
func (b *Broker[T]) MustDeliverDropCount() uint64 {
	return b.mustDeliverDropCount.Load()
}

// Publish 非阻塞发布：Channel 满时丢弃并递增 dropCount
func (b *Broker[T]) Publish(event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-b.done:
		return
	default:
	}

	for sub := range b.subs {
		select {
		case sub <- event:
		default:
			b.dropCount.Add(1)
			slog.Warn("Pubsub buffer full; dropping event")
		}
	}
}

// PublishMustDeliver 有界阻塞发布：先尝试非阻塞，失败后阻塞等待
// 最多等待 mustDeliverTimeout（默认 50ms），超时则丢弃
func (b *Broker[T]) PublishMustDeliver(ctx context.Context, event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-b.done:
		return
	default:
	}

	timeout := b.mustDeliverTimeout

	for sub := range b.subs {
		// Fast path: non-blocking send
		select {
		case sub <- event:
			continue
		default:
		}

		// Slow path: bounded blocking send
		timer := time.NewTimer(timeout)
		select {
		case sub <- event:
			timer.Stop()
		case <-timer.C:
			b.mustDeliverDropCount.Add(1)
			slog.Error("PublishMustDeliver timed out", "timeout", timeout)
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

// Shutdown 优雅关闭：关闭 done channel，清理所有订阅者
func (b *Broker[T]) Shutdown() {
	select {
	case <-b.done:
		return // 已关闭
	default:
		close(b.done)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs {
		delete(b.subs, sub)
		close(sub)
	}
	b.subCount = 0
}
