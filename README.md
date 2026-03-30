---
title: Heinzel AI Addons (Go)
aliases: [heinzel-addons-go]
tags:
  - heinzel
  - addons
type: spec
status: active
created: 2026-03-29
modified: 2026-03-30
project: heinzel-ai-addons-go
---

# Heinzel AI Addons

Official addon collection for the Heinzel AI agent framework.

## What it is

39 addons that turn the bare Heinzel core into a full agent: CLI, Web GUI, reasoning, memory, tools, cost control, security, voice, authentication, logging, and more. Each addon is independent -- load what you need, leave the rest.

## Addons

### IO
| Addon | Description |
|-------|-------------|
| CLI | Interactive terminal with history, RC-files, startup docs |
| WebGUI | Web chat interface with Markdown, Mermaid, SSE streaming |
| Voice | Whisper STT + TTS (cloud or local) |

### Reasoning
| Addon | Description |
|-------|-------------|
| Reasoning | Strategy selection, LLM triage, multi-step thinking |
| ReasoningPlan | Step-by-step task planning with approval flow |
| ReasoningThink | Agent-controlled thinking (decompose -> analyze -> evaluate -> synthesize) |

### Memory
| Addon | Description |
|-------|-------------|
| MemoryComposer | Orchestrates memory layers (Facts, Recent, Summary, Tools) |
| Memory (Cognitive) | Facade over Prolog, Scripts, Vault -- own LLM channel |
| Compaction | Context compaction via fact extraction and summarization |

### Tools
| Addon | Description |
|-------|-------------|
| MCPManager | Dynamic MCP server load/unload (HTTP + Stdio) |
| MCPClient | Stdio MCP transport |
| MCPHTTPClient | HTTP MCP transport (Streamable HTTP) |
| MCPPermissions | Tool permission system (allow/deny/ask, glob patterns) |
| FileUpload | @path syntax for file and image injection |
| WebSearch | Web search via SearXNG and URL content fetching |
| Shell | Shell command execution with sandboxing |
| Browser | Headless browser control (Rod) |
| DB | SQLite/PostgreSQL database addon |

### Control
| Addon | Description |
|-------|-------------|
| Commands | Universal slash-command handler (/help, /clear, /status, ...) |
| CostGuard | Token budget enforcement, cost tracking, per-session budgets |
| Guard | ExecutionGuard -- 3 modes: paranoid, normal, expert |
| Recovery | Error handling, circuit breaker, retry classification |
| PromptAddon | Prompt composition, awareness injection, skills |
| Workflows | Multi-step workflow execution (YAML-defined) |
| Feedback | User feedback collection and routing |

### Auth
| Addon | Description |
|-------|-------------|
| Auth | Authentication and session management |
| Passkey | WebAuthn/FIDO2 passwordless authentication |

### Logging
| Addon | Description |
|-------|-------------|
| ChatLog | Session log to disk (JSON) |
| Transcript | Numbered turn protocol with search and context injection |
| Logger | Debug observer for hook dispatches |

### Provider
| Addon | Description |
|-------|-------------|
| HTTPProvider | The LLM provider -- talks to external provider service |
| EchoProvider | Test provider that echoes input |

## Usage

```go
import (
    "github.com/cuber-it/heinzel-ai-core-go/core"
    "github.com/cuber-it/heinzel-ai-addons-go/addons"
)

dispatcher := core.NewDispatcher()
dispatcher.Register(addons.NewHTTPProvider("openai", "http://localhost:12101", "gpt-4.1"), 100)
dispatcher.Register(addons.NewCommandAddon(dispatcher), 1)
dispatcher.Register(addons.NewCostGuardAddon(), 2)
// ... add what you need
```

## Tests

245 tests covering all addons.

```bash
go test ./...
```

## Platform Support

Some addons include platform-specific files (e.g., audio, browser). Build tags are used where necessary.

## Related

- [heinzel-ai-core-go](https://github.com/cuber-it/heinzel-ai-core-go) -- The engine
- [heinzel-ai-addons-py](https://github.com/cuber-it/heinzel-ai-addons-py) -- Python version
- [heinzel-assistant](https://github.com/cuber-it/heinzel-assistant) -- Consumer agent ("Your Personal Heinzel")
- [heinzel-crew](https://github.com/cuber-it/heinzel-crew) -- Multi-agent team (includes Mattermost addon)

## License

Apache 2.0 -- see [LICENSE](LICENSE)

Built with assistance from Claude (Anthropic).
