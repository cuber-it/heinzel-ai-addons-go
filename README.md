# Heinzel AI Addons

Official addon collection for the Heinzel AI agent framework.

## What it is

30 addons that turn the bare Heinzel core into a full agent: CLI, Web GUI, reasoning, memory, tools, cost control, logging, and more. Each addon is independent — load what you need, leave the rest.

## Addons

### IO
| Addon | Description |
|-------|-------------|
| CLI | Interactive terminal with history, RC-files, startup docs |
| WebGUI | Web chat interface with Markdown, Mermaid, SSE streaming |

### Reasoning
| Addon | Description |
|-------|-------------|
| Reasoning | Strategy selection, LLM triage, multi-step thinking |
| ReasoningPlan | Step-by-step task planning with approval flow |
| ReasoningThink | Agent-controlled thinking (decompose → analyze → evaluate → synthesize) |

### Memory
| Addon | Description |
|-------|-------------|
| MemoryComposer | Orchestrates memory layers (Facts, Recent, Summary, Tools) |
| Memory (Cognitive) | Facade over Prolog, Scripts, Vault — own LLM channel |
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

### Control
| Addon | Description |
|-------|-------------|
| Commands | Universal slash-command handler (/help, /clear, /status, ...) |
| CostGuard | Token budget enforcement and cost tracking |
| Recovery | Error handling, circuit breaker, retry classification |
| PromptAddon | Prompt composition, awareness injection, skills |

### Logging
| Addon | Description |
|-------|-------------|
| ChatLog | Session log to disk (JSON) |
| Transcript | Numbered turn protocol with search and context injection |
| Logger | Debug observer for hook dispatches |

### Provider
| Addon | Description |
|-------|-------------|
| HTTPProvider | The LLM provider — talks to external provider service |
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

## Related

- [heinzel-ai-core-go](https://github.com/cuber-it/heinzel-ai-core-go) — The engine
- [heinzel-crew](https://github.com/cuber-it/heinzel-crew) — Multi-agent team (includes Mattermost addon)

## License

Apache 2.0 — see [LICENSE](LICENSE)
