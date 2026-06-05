package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"AICodeAgent/internal/agent"
	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/config"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/pubsub"
)

// App 依赖注入容器，管理所有共享组件
type App struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	Config    *config.Config
	Logger    *slog.Logger

	Coordinator    agent.Coordinator
	SessionService agent.SessionService
	ToolRegistry   *tools.ToolRegistry
	PermService    *permission.PermissionService
	Broker         *pubsub.Broker[events.Event]
}

// New 创建 App 实例并初始化所有组件
func New(ctx context.Context, cfg *config.Config) *App {
	appCtx, cancel := context.WithCancel(ctx)
	logger := slog.Default()

	// 创建内存存储
	cache := newMemoryCache()
	storage := newMemoryStorage()
	locker := newMemoryLocker()

	// 初始化会话服务
	idleTimeout, _ := time.ParseDuration(cfg.Session.IdleTimeout)
	if idleTimeout == 0 {
		idleTimeout = 30 * time.Minute
	}
	sessionCfg := agent.SessionConfig{
		IdleTimeout: idleTimeout,
		MaxSessions: cfg.Session.MaxSessions,
	}
	sessionService := agent.NewSessionService(cache, storage, locker, sessionCfg)

	// 初始化消息服务（内存实现）
	messageService := newMemoryMessageService()

	// 初始化工具注册表
	toolRegistry := tools.NewToolRegistry()
	registerDefaultTools(toolRegistry)

	// 初始化权限服务
	permLevel := permission.PermLevel(cfg.Permission.DefaultLevel)
	permService := permission.NewPermissionService(
		permLevel,
		cfg.Permission.FileWhitelist,
		cfg.Permission.CmdWhitelist,
	)

	// 初始化事件总线
	broker := pubsub.NewBroker[events.Event]()

	// 初始化Coordinator
	coordinator := agent.NewCoordinator(
		cfg,
		sessionService,
		messageService,
		toolRegistry,
		permService,
		broker,
	)

	app := &App{
		Ctx:            appCtx,
		Cancel:         cancel,
		Config:         cfg,
		Logger:         logger,
		Coordinator:    coordinator,
		SessionService: sessionService,
		ToolRegistry:   toolRegistry,
		PermService:    permService,
		Broker:         broker,
	}

	logger.Info("App初始化完成")
	return app
}

// Close 优雅关闭程序
func (a *App) Close() {
	if a.Coordinator != nil {
		a.Coordinator.CancelAll()
	}
	if a.Broker != nil {
		a.Broker.Shutdown()
	}
	a.Cancel()
	a.Logger.Info("程序已优雅关闭")
}

// StartEventLoop 启动事件循环：订阅 Broker，将 UserMessage 分发给 Coordinator
func (a *App) StartEventLoop(ctx context.Context) {
	ch := a.Broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				switch e := event.(type) {
				case events.UserMessage:
					a.Coordinator.HandleUserMessage(e)
				}
			}
		}
	}()
}

// registerDefaultTools 注册默认工具
func registerDefaultTools(registry *tools.ToolRegistry) {
	registry.Register(&tools.ToolMeta{
		ID:           "file_read",
		Name:         "读取文件",
		Type:         tools.ToolTypeFile,
		Description:  "读取指定路径的文件内容",
		Params:       []tools.ParamMeta{{Name: "path", Type: "string", Required: true}},
		MinPermLevel: int(permission.PermLevelNormal),
		Enabled:      true,
	})
	registry.Register(&tools.ToolMeta{
		ID:           "file_write",
		Name:         "写入文件",
		Type:         tools.ToolTypeFile,
		Description:  "向指定路径写入内容",
		Params:       []tools.ParamMeta{{Name: "path", Type: "string", Required: true}, {Name: "content", Type: "string", Required: true}},
		MinPermLevel: int(permission.PermLevelAdvanced),
		Enabled:      true,
	})
	registry.Register(&tools.ToolMeta{
		ID:           "terminal_exec",
		Name:         "执行命令",
		Type:         tools.ToolTypeTerminal,
		Description:  "在终端中执行命令",
		Params:       []tools.ParamMeta{{Name: "command", Type: "string", Required: true}},
		MinPermLevel: int(permission.PermLevelAdmin),
		Enabled:      true,
	})
}

// memoryCache 内存缓存实现
type memoryCache struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func newMemoryCache() *memoryCache {
	return &memoryCache{data: make(map[string][]byte)}
}

func (m *memoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("cache miss: %s", key)
	}
	return data, nil
}

func (m *memoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memoryCache) Del(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// memoryStorage 内存持久化存储实现
type memoryStorage struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{data: make(map[string][]byte)}
}

func (m *memoryStorage) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("storage miss: %s", key)
	}
	return data, nil
}

func (m *memoryStorage) Put(ctx context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memoryStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memoryStorage) List(ctx context.Context, prefix string) ([][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results [][]byte
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			results = append(results, v)
		}
	}
	return results, nil
}

// memoryLocker 内存锁实现
type memoryLocker struct {
	locks map[string]*memoryLock
	mu    sync.Mutex
}

func newMemoryLocker() *memoryLocker {
	return &memoryLocker{locks: make(map[string]*memoryLock)}
}

func (m *memoryLocker) Lock(ctx context.Context, key string, ttl time.Duration) (agent.Lock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.locks[key]; exists {
		return nil, fmt.Errorf("lock already held: %s", key)
	}
	lock := &memoryLock{key: key, locker: m}
	m.locks[key] = lock
	return lock, nil
}

type memoryLock struct {
	key    string
	locker *memoryLocker
}

func (l *memoryLock) Unlock(ctx context.Context) error {
	l.locker.mu.Lock()
	defer l.locker.mu.Unlock()
	delete(l.locker.locks, l.key)
	return nil
}

// memoryMessageService 内存消息服务实现
type memoryMessageService struct {
	messages map[string][]agent.Message
	mu       sync.RWMutex
}

func newMemoryMessageService() *memoryMessageService {
	return &memoryMessageService{messages: make(map[string][]agent.Message)}
}

func (m *memoryMessageService) List(ctx context.Context, sessionID string) ([]agent.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.messages[sessionID], nil
}

func (m *memoryMessageService) Create(ctx context.Context, sessionID string, params agent.CreateMessageParams) (*agent.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg := &agent.Message{
		Role:    params.Role,
		Content: params.Content,
	}
	m.messages[sessionID] = append(m.messages[sessionID], *msg)
	return msg, nil
}
