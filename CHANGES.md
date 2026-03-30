# Changes

## 0.1.0 — 2026-03-29

Initial release. 30 addons extracted from neo-heinzel monolith.

### IO
- CLI Bridge with history, RC-files, startup docs, bash execution
- WebGUI with Markdown, Mermaid, SSE streaming, file upload

### Reasoning
- Strategy selection with LLM triage (auto-classifies input)
- Multi-step agent-controlled thinking (decompose → analyze → evaluate → challenge)
- Steerable depth (0-5) per strategy
- Plan mode with step-by-step execution and approval

### Memory
- MemoryComposer with 4 layers (Facts, Recent, Summary, Tools)
- Cognitive Memory facade (Prolog/Scripts/Vault via own LLM channel)
- Context compaction (extract facts, summarize, rolling handover)
- MCP-based compaction sink

### Tools
- MCP Manager (HTTP + Stdio, dynamic load/unload)
- MCP permissions (allow/deny/ask, glob patterns, persistent YAML)
- File upload (@path syntax, image detection, base64)
- Web search (SearXNG, URL fetch, html2text)

### Control
- Universal slash-command handler
- CostGuard (token budgets, cost limits, passphrase override)
- Recovery (circuit breaker, retry classification, graceful degradation)
- Prompt composition + awareness + skills

### Logging
- ChatLog (JSON session persistence)
- Transcript (numbered turns, search, context injection)
- Logger (debug hook observer)
