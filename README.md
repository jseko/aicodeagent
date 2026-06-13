# AICodeAgent

**Terminal-native AI coding agent built with Go**

AICodeAgent is a production-grade, terminal-based AI coding assistant. It combines an event-driven architecture, LLM-powered agent loop, MCP protocol integration, LSP diagnostics, and a modern TUI — all built from scratch in Go.

## Installation

### macOS (Homebrew)

```bash
brew tap jseko/homebrew-tap
brew install aicodeagent
```

### Linux (deb/rpm/apk)

Download the latest package from [GitHub Releases](https://github.com/jseko/aicodeagent/releases).

### Go Install

```bash
go install github.com/jseko/aicodeagent/cmd/AICodeAgent@latest
```

## Quick Start

```bash
# Interactive configuration wizard
aicodeagent init

# Start TUI mode
aicodeagent

# Single-shot chat
aicodeagent chat "Explain this codebase"

# View logs
aicodeagent logs --follow

# Generate shell completions
aicodeagent completion bash > ~/.bash_completion.d/aicodeagent
```

## Configuration

Config file: `~/.aicode/config.yaml` (global) or `./config.yaml` (project).

Priority (low to high):
```
defaults → ./config.yaml → ~/.aicode/config.yaml → environment variables
```

```yaml
theme: dark
log_level: info

providers:
  - type: openai
    name: openai
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}

models:
  default: gpt-4o
  items:
    gpt-4o:
      provider: openai
      model: gpt-4o
      temperature: 0.7
      max_tokens: 4096
```

## Architecture

```
┌─────────────────────────────────────────────┐
│              Terminal UI (Bubble Tea)        │
└─────────────────┬───────────────────────────┘
                  │ Pub/Sub Events
┌─────────────────▼───────────────────────────┐
│           Coordinator (ReAct Loop)           │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ Session  │ │  Agent   │ │  Provider   │  │
│  └──────────┘ └──────────┘ └─────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ Message  │ │   Tool   │ │ Permission  │  │
│  └──────────┘ └──────────┘ └─────────────┘  │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│            Agent Runtime                     │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │   LLM    │ │   Tool   │ │    LSP/MCP  │  │
│  │ Provider │ │ Executor │ │   Clients   │  │
│  └──────────┘ └──────────┘ └─────────────┘  │
└─────────────────────────────────────────────┘
```

## Features

| Module | Description |
|--------|-------------|
| **Agent** | ReAct loop with streaming LLM responses |
| **Tools** | Native tools (view, write, bash, diagnostics) + MCP integration |
| **LSP** | Language Server Protocol client for code intelligence |
| **MCP** | Model Context Protocol with STDIO/HTTP transport |
| **TUI** | Modern terminal UI with real-time streaming, themes |
| **Skills** | Agent skill system with discovery and slash-completion |
| **Subagent** | Task-specific sub-agents with permission scoping |
| **Rules** | Rule engine with exact/regex/semantic matching |
| **Memory** | Context memory with conversation summarization |
| **Hooks** | External hook system for event-driven customization |
| **Pub/Sub** | Type-safe generic event bus |
| **Logging** | Structured logging (slog + lumberjack rotation) |

## Commands

```
AICodeAgent              Start interactive TUI
AICodeAgent init         Interactive configuration wizard
AICodeAgent chat <msg>   Single-shot conversation
AICodeAgent logs [-f]    View/tail log files
AICodeAgent completion   Generate shell completions (bash/zsh/fish)
AICodeAgent help         Show help
```

## Development

```bash
# Build
go build -race ./cmd/AICodeAgent

# Test
go test -race -coverprofile=coverage.out ./...

# Lint
golangci-lint run --config=.golangci.yml
```

## License

MIT
