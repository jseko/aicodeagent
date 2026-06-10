package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SessionStatus 会话状态
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusIdle     SessionStatus = "idle"
	SessionStatusArchived SessionStatus = "archived"

	sessionKeyPrefix     = "session:"
	sessionLockKeyFormat = "session:lock:%s"
	sessionIDFormat      = "sess_%d_%s"
	lockTTL              = 5 * time.Second
)

// Session 会话数据模型
type Session struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Status    SessionStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`

	// 当前上下文 Token 统计，摘要压缩后可重置
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`

	// 累计账单 Token 统计，不随摘要压缩重置
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalCost             float64 `json:"total_cost"`

	// 总结消息ID
	SummaryMessageID string `json:"summary_message_id,omitempty"`

	// 待办事项
	Todos []string `json:"todos,omitempty"`

	// 是否禁用自动总结
	DisableAutoSummarize bool `json:"disable_auto_summarize"`

	// Revert 回滚点（Undo时设置，Redo时清空）
	Revert *RevertPoint `json:"revert,omitempty"`
}

// RevertPoint Undo/Redo 回滚点
type RevertPoint struct {
	MessageID string    `json:"message_id"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionService 会话服务接口
type SessionService interface {
	Get(ctx context.Context, id string) (*Session, error)
	Create(ctx context.Context, title string) (*Session, error)
	Save(ctx context.Context, session *Session) error
	List(ctx context.Context) ([]*Session, error)
	Delete(ctx context.Context, id string) error
}

// CacheClient 缓存客户端接口
type CacheClient interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Time) error
	Del(ctx context.Context, key string) error
}

// StorageClient 持久化存储客户端接口
type StorageClient interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([][]byte, error)
}

// LockClient 分布式锁客户端接口
type LockClient interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}

// Lock 锁接口
type Lock interface {
	Unlock(ctx context.Context) error
}

// sessionService 会话服务实现（二级存储：内存 + 磁盘）
type sessionService struct {
	cacheClient   CacheClient
	storageClient StorageClient
	lockClient    LockClient
	config        SessionConfig
	mu            sync.RWMutex
}

// SessionConfig 会话配置
type SessionConfig struct {
	IdleTimeout time.Duration
	MaxSessions int
}

// NewSessionService 创建会话服务
func NewSessionService(cache CacheClient, storage StorageClient, lock LockClient, cfg SessionConfig) SessionService {
	return &sessionService{
		cacheClient:   cache,
		storageClient: storage,
		lockClient:    lock,
		config:        cfg,
	}
}

// Get 获取会话（先查缓存，再查存储）
func (s *sessionService) Get(ctx context.Context, id string) (*Session, error) {
	// 1. 查缓存
	if data, err := s.cacheClient.Get(ctx, sessionKeyPrefix+id); err == nil && data != nil {
		var session Session
		if err := json.Unmarshal(data, &session); err == nil {
			return &session, nil
		}
	}

	// 2. 回源存储
	data, err := s.storageClient.Get(ctx, sessionKeyPrefix+id)
	if err != nil {
		return nil, fmt.Errorf("get session failed: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}

// Create 创建新会话
func (s *sessionService) Create(ctx context.Context, title string) (*Session, error) {
	session := &Session{
		ID:        generateID(),
		Title:     title,
		Status:    SessionStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.Save(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// Save 写透策略持久化：先写磁盘，再写缓存
func (s *sessionService) Save(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()
	sessionBytes, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	// 1. 先写入持久化存储（数据安全优先）
	if err := s.storageClient.Put(ctx, sessionKeyPrefix+session.ID, sessionBytes); err != nil {
		return fmt.Errorf("storage put: %w", err)
	}

	// 2. 再更新内存缓存（提升读取性能）
	return s.cacheClient.Set(ctx, sessionKeyPrefix+session.ID, sessionBytes, session.ExpiresAt)
}

// List 列出所有活跃会话
func (s *sessionService) List(ctx context.Context) ([]*Session, error) {
	datas, err := s.storageClient.List(ctx, sessionKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []*Session
	for _, data := range datas {
		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		if session.Status != SessionStatusArchived {
			sessions = append(sessions, &session)
		}
	}
	return sessions, nil
}

// Delete 删除会话
func (s *sessionService) Delete(ctx context.Context, id string) error {
	if err := s.storageClient.Delete(ctx, sessionKeyPrefix+id); err != nil {
		return err
	}
	return s.cacheClient.Del(ctx, sessionKeyPrefix+id)
}

// UpdateSessionStatus 带锁的状态更新
func (s *sessionService) UpdateSessionStatus(ctx context.Context, sessionID string, status SessionStatus) error {
	lockKey := fmt.Sprintf(sessionLockKeyFormat, sessionID)
	lock, err := s.lockClient.Lock(ctx, lockKey, lockTTL)
	if err != nil {
		return fmt.Errorf("acquire lock failed: %w", err)
	}
	defer lock.Unlock(ctx)

	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session failed: %w", err)
	}

	if !isValidStatusTransition(session.Status, status) {
		return fmt.Errorf("invalid status transition: %s -> %s", session.Status, status)
	}

	session.Status = status
	if status == SessionStatusIdle {
		session.ExpiresAt = time.Now().Add(s.config.IdleTimeout)
	} else if status == SessionStatusActive {
		session.ExpiresAt = time.Time{}
	}

	return s.Save(ctx, session)
}

// isValidStatusTransition 校验状态转换合法性
func isValidStatusTransition(from, to SessionStatus) bool {
	switch from {
	case SessionStatusActive:
		return to == SessionStatusIdle || to == SessionStatusArchived
	case SessionStatusIdle:
		return to == SessionStatusActive || to == SessionStatusArchived
	case SessionStatusArchived:
		return false // 归档不可逆
	default:
		return false
	}
}

// generateID 生成唯一会话ID（纳秒时间戳 + 随机后缀防止碰撞）
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf(sessionIDFormat, time.Now().UnixNano(), hex.EncodeToString(b))
}
