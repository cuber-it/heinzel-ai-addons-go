---
title: Heinzel AI Addons (Go) Changes
tags:
  - heinzel
  - addons
  - changelog
type: note
status: active
created: 2026-03-29
modified: 2026-03-30
project: heinzel-ai-addons-go
---

# Changes

## 0.1.0 -- 2026-03-30

Initial release. 39 addons extracted from neo-heinzel monolith. 245 tests.

### IO
- CLI Bridge with history, RC-files, startup docs, bash execution
- WebGUI with Markdown, Mermaid, SSE streaming, file upload
- Voice: Whisper STT + TTS (cloud or local)

### Reasoning
- Strategy selection with LLM triage (auto-classifies input)
- Multi-step agent-controlled thinking (decompose -> analyze -> evaluate -> challenge)
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
- Shell command execution with sandboxing
- Browser control (Rod, headless)
- DB addon (SQLite/PostgreSQL)

### Control
- Universal slash-command handler
- CostGuard: token budgets, cost limits, per-session budgets, passphrase override
- Guard: ExecutionGuard with 3 modes (paranoid, normal, expert)
- Recovery (circuit breaker, retry classification, graceful degradation)
- Prompt composition + awareness + skills
- Workflows: multi-step YAML-defined workflow execution
- Feedback: user feedback collection and routing

### Auth
- Authentication and session management
- Passkey: WebAuthn/FIDO2 passwordless authentication

### Logging
- ChatLog (JSON session persistence)
- Transcript (numbered turns, search, context injection)
- Logger (debug hook observer)

### Other
- Clean code pass: consistent naming, reduced duplication
- Platform-specific files for audio and browser (build tags)
