package subagent

import (
	"fmt"
	"sync"
	"time"

	"AICodeAgent/internal/llm"
)

const (
	SessionStatusRunning = "running"
	SessionStatusDone    = "done"
	SessionStatusFailed  = "failed"
)

type Message struct {
	Role    string
	Content string
	At      time.Time
}

type SubagentContext struct {
	SessionID    string
	ParentID     string
	SystemPrompt string
	Tools        []string
	History      []Message
	CreatedAt    time.Time
}

func NewSubagentContext(parentID string, agent *Subagent) *SubagentContext {
	return &SubagentContext{
		SessionID:    generateSessionID(),
		ParentID:     parentID,
		SystemPrompt: agent.SystemPrompt,
		Tools:        filterTools(agent.Config.Tools),
		History:      make([]Message, 0),
		CreatedAt:    time.Now(),
	}
}

func (c *SubagentContext) AddMessage(role, content string) {
	c.History = append(c.History, Message{Role: role, Content: content, At: time.Now()})
}

func (c *SubagentContext) BuildPrompt(userInput string) string {
	return c.SystemPrompt + "\n\n" + userInput
}

func (c *SubagentContext) BuildMessages(userInput string) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: c.SystemPrompt}}
	for _, msg := range c.History {
		messages = append(messages, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	if userInput != "" {
		messages = append(messages, llm.Message{Role: "user", Content: userInput})
	}
	return messages
}

type SubagentSession struct {
	ID               string
	ParentID         string
	SubagentName     string
	Status           string
	History          []Message
	PromptTokens     int64
	CompletionTokens int64
	Cost             float64
	UpdatedAt        time.Time
}

func (s *SubagentSession) RecordTokenUsage(modelName string, promptTokens, completionTokens int64) {
	s.PromptTokens += promptTokens
	s.CompletionTokens += completionTokens
	s.UpdatedAt = time.Now()
}

type SubagentCoordinator struct {
	registry     *SubagentRegistry
	current      *Subagent
	context      *SubagentContext
	sessionStack []string
	sessions     map[string]*SubagentSession
	mu           sync.RWMutex
}

func NewCoordinator(registry *SubagentRegistry) *SubagentCoordinator {
	if registry == nil {
		registry = NewRegistry()
	}
	return &SubagentCoordinator{
		registry:     registry,
		sessionStack: make([]string, 0),
		sessions:     make(map[string]*SubagentSession),
	}
}

func (c *SubagentCoordinator) SwitchRole(parentID, name string) (*SubagentContext, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	agent, err := c.registry.Get(name)
	if err != nil {
		return nil, fmt.Errorf("subagent not found: %w", err)
	}
	ctx := NewSubagentContext(parentID, agent)
	c.current = agent
	c.context = ctx
	c.sessionStack = append(c.sessionStack, ctx.SessionID)
	c.sessions[ctx.SessionID] = &SubagentSession{
		ID:           ctx.SessionID,
		ParentID:     parentID,
		SubagentName: name,
		Status:       SessionStatusRunning,
		History:      ctx.History,
		UpdatedAt:    time.Now(),
	}
	return ctx, nil
}

func (c *SubagentCoordinator) RestoreMainAgent() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return fmt.Errorf("no subagent to restore")
	}
	if len(c.sessionStack) > 0 {
		c.sessionStack = c.sessionStack[:len(c.sessionStack)-1]
	}
	c.current = nil
	c.context = nil
	return nil
}

func (c *SubagentCoordinator) MarkDone(id string) {
	c.markStatus(id, SessionStatusDone)
}

func (c *SubagentCoordinator) MarkFailed(id string) {
	c.markStatus(id, SessionStatusFailed)
}

func (c *SubagentCoordinator) markStatus(id, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, ok := c.sessions[id]
	if !ok {
		return
	}
	session.Status = status
	session.UpdatedAt = time.Now()
}

func (c *SubagentCoordinator) Current() (*Subagent, *SubagentContext) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current, c.context
}

func (c *SubagentCoordinator) ResumeSession(id string) (*SubagentSession, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, ok := c.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	if session.Status != SessionStatusRunning {
		return nil, fmt.Errorf("session not running")
	}
	return session, nil
}

func filterTools(tools map[string]bool) []string {
	out := make([]string, 0, len(tools))
	for name, enabled := range tools {
		if enabled {
			out = append(out, name)
		}
	}
	return out
}

func generateSessionID() string {
	return fmt.Sprintf("subagent-%d", time.Now().UnixNano())
}
