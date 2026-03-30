// Cognitive memory addon — connects Prolog, Vault, and Forth backends.
// Stub implementation — wire up actual MCP backends in a follow-up.

package addons

import (
	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// MemoryConfig holds endpoints for the cognitive memory backends
type MemoryConfig struct {
	LLMEndpoint string
	LLMModel    string
	PrologURL   string
	VaultURL    string
	ScriptURL   string
}

// MemoryAddon orchestrates cognitive memory via external MCP backends
type MemoryAddon struct {
	core.BaseAddon
	config MemoryConfig
}

func NewMemoryAddon(config MemoryConfig) *MemoryAddon {
	return &MemoryAddon{config: config}
}

func (addon *MemoryAddon) Name() string           { return "cognitive_memory" }
func (addon *MemoryAddon) Type() core.AddonType   { return core.AddonMemory }
func (addon *MemoryAddon) Hooks() []core.HookPoint { return nil }
func (addon *MemoryAddon) Start() error            { return nil }
func (addon *MemoryAddon) Stop() error             { return nil }

func (addon *MemoryAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	return core.Result{}
}
