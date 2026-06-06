package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Server states
const (
	StateDisabled  = "disabled"
	StateStarting  = "starting"
	StateConnected = "connected"
	StateError     = "error"
)

const maxReconnectRetries = 3

// ServerState tracks MCP server state
type ServerState struct {
	Name   string
	State  string
	Error  error
	Counts Counts
}

// Counts tracks tool count and retry count
type Counts struct {
	Tools   int
	Retries int
}

var (
	sessions sync.Map // map[string]*ClientSession
	states   sync.Map // map[string]*ServerState
	configs  sync.Map // map[string]MCPConfigAdapter
)

// updateState updates the state tracking for an MCP server
func updateState(name, state string, err error, _ *ClientSession, counts Counts) {
	s := &ServerState{
		Name:   name,
		State:  state,
		Error:  err,
		Counts: counts,
	}
	states.Store(name, s)
}

// getOrRenewClient gets an existing session or reconnects if the connection is lost
func getOrRenewClient(ctx context.Context, name string) (*ClientSession, error) {
	val, ok := sessions.Load(name)
	if !ok {
		return nil, fmt.Errorf("mcp '%s' not available", name)
	}
	sess := val.(*ClientSession)

	// Ping health check
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := sess.Ping(pingCtx); err == nil {
		return sess, nil
	}

	// Connection lost, update state
	updateState(name, StateError, fmt.Errorf("connection lost"), nil, Counts{})

	cfgVal, ok := configs.Load(name)
	if !ok {
		return nil, fmt.Errorf("mcp '%s' config not found for reconnect", name)
	}
	cfg := cfgVal.(MCPConfigAdapter)

	// Check retry limit
	stateVal, _ := states.Load(name)
	s := stateVal.(*ServerState)
	if s.Counts.Retries >= maxReconnectRetries {
		return nil, fmt.Errorf("mcp '%s' max reconnect retries (%d) exceeded", name, maxReconnectRetries)
	}

	// Exponential backoff
	backoff := time.Duration(1<<s.Counts.Retries) * time.Second
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(backoff):
	}

	// Attempt reconnect
	newSess, err := createSession(ctx, name, cfg)
	if err != nil {
		s.Counts.Retries++
		updateState(name, StateError, err, nil, s.Counts)
		return nil, fmt.Errorf("reconnect failed: %w", err)
	}

	// Re-discover tools after reconnect
	if _, err := newSess.ListTools(ctx); err != nil {
		newSess.Close()
		s.Counts.Retries++
		updateState(name, StateError, err, nil, s.Counts)
		return nil, fmt.Errorf("reconnect list tools: %w", err)
	}

	sessions.Store(name, newSess)
	updateState(name, StateConnected, nil, newSess, Counts{Tools: s.Counts.Tools})
	log.Printf("[MCP] Server %s reconnected successfully", name)
	return newSess, nil
}

// GetAllTools returns all tools from all connected MCP servers
func GetAllTools(clients map[string]*ClientSession) []*Tool {
	var allTools []*Tool
	for _, sess := range clients {
		allTools = append(allTools, sess.Tools()...)
	}
	return allTools
}

// Initialize concurrently initializes all configured MCP servers
func Initialize(ctx context.Context, cfgs map[string]MCPConfigAdapter) {
	var wg sync.WaitGroup

	for name, cfg := range cfgs {
		if cfg.Disabled {
			updateState(name, StateDisabled, nil, nil, Counts{})
			log.Printf("[MCP] Server %s is disabled, skipping", name)
			continue
		}

		wg.Add(1)
		go func(name string, cfg MCPConfigAdapter) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					updateState(name, StateError, fmt.Errorf("panic: %v", r), nil, Counts{})
					log.Printf("[MCP] Server %s panicked during init: %v", name, r)
				}
			}()

			updateState(name, StateStarting, nil, nil, Counts{})

			// Store config for later reconnection
			configs.Store(name, cfg)

			session, err := createSession(ctx, name, cfg)
			if err != nil {
				updateState(name, StateError, err, nil, Counts{})
				log.Printf("[MCP] Server %s init failed: %v", name, err)
				return
			}

			tools, err := session.ListTools(ctx)
			if err != nil {
				session.Close()
				updateState(name, StateError, err, nil, Counts{})
				log.Printf("[MCP] Server %s list tools failed: %v", name, err)
				return
			}

			sessions.Store(name, session)
			updateState(name, StateConnected, nil, session, Counts{Tools: len(tools)})
			log.Printf("[MCP] Server %s initialized with %d tools", name, len(tools))
		}(name, cfg)
	}

	wg.Wait()
}

// Close gracefully shuts down all MCP connections
func Close() {
	var wg sync.WaitGroup

	sessions.Range(func(key, value any) bool {
		wg.Add(1)
		go func(name string, sess *ClientSession) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[MCP] Server %s panicked during close: %v", name, r)
				}
			}()

			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				sess.Close()
				close(done)
			}()

			select {
			case <-closeCtx.Done():
				log.Printf("[MCP] Server %s close timeout", name)
			case <-done:
				log.Printf("[MCP] Server %s closed gracefully", name)
			}
		}(key.(string), value.(*ClientSession))
		return true
	})

	wg.Wait()
	log.Println("[MCP] All servers closed")
}

// GetSession returns the session for a given server name
func GetSession(name string) (*ClientSession, bool) {
	val, ok := sessions.Load(name)
	if !ok {
		return nil, false
	}
	return val.(*ClientSession), true
}

// GetState returns the state for a given server name
func GetState(name string) (*ServerState, bool) {
	val, ok := states.Load(name)
	if !ok {
		return nil, false
	}
	return val.(*ServerState), true
}

// ListSessions returns all active MCP client sessions
func ListSessions() map[string]*ClientSession {
	result := make(map[string]*ClientSession)
	sessions.Range(func(key, value any) bool {
		result[key.(string)] = value.(*ClientSession)
		return true
	})
	return result
}
