// Addon builder registration — connects addon constructors to the factory.

package addons

import (
	"os"
	"path/filepath"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const (
	defaultMemoryBudget   = 16000 // memory composer token budget
	defaultSummaryTokens  = 2000  // session summary token limit
	defaultRecentMessages = 50    // recent messages to keep
	defaultMaxBacktracks  = 3     // reasoning backtracks
	defaultMaxUploadMB    = 10    // file upload limit in MB
)

func init() {
	// Pflicht
	core.RegisterAddonBuilder("cli", buildCLI)
	core.RegisterAddonBuilder("logger", buildLogger)
	core.RegisterAddonBuilder("prompt", buildPrompt)
	core.RegisterAddonBuilder("recovery", buildRecovery)
	core.RegisterAddonBuilder("commands", buildCommands)
	core.RegisterAddonBuilder("costguard", buildCostGuard)
	core.RegisterAddonBuilder("provider", buildProvider)

	// Optional
	core.RegisterAddonBuilder("reasoning", buildReasoning)
	core.RegisterAddonBuilder("memory_composer", buildMemoryComposer)
	core.RegisterAddonBuilder("compaction", buildCompaction)
	core.RegisterAddonBuilder("file_upload", buildFileUpload)
	core.RegisterAddonBuilder("websearch", buildWebSearch)
	core.RegisterAddonBuilder("chatlog", buildChatLog)
	core.RegisterAddonBuilder("transcript", buildTranscript)
	core.RegisterAddonBuilder("mcp_manager", buildMCPManager)
	core.RegisterAddonBuilder("heartbeat", buildHeartbeat)
	core.RegisterAddonBuilder("cognitive_memory", buildCognitiveMemory)
}

// --- Pflicht Builders ---

func buildCLI(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	bridge := NewCLIBridge("> ", config.Logging.Thinking || config.Logging.Verbose)
	bridge.SetStartupDocs(config.StartupDocs)
	core.NewIORegistry(bridge)
	return bridge, 0, nil
}

func buildLogger(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewLogger(config.Logging.Verbose), 1, nil
}

func buildPrompt(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewPromptAddon(config.Prompts.System, dispatcher, config.Prompts.SkillDirs), 5, nil
}

func buildRecovery(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewRecoveryAddon(), 0, nil
}

func buildCommands(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewCommandAddon(dispatcher), 1, nil
}

func buildCostGuard(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	costGuard := NewCostGuardAddon()
	if config.CostGuard.MaxTokensPerTurn > 0 || config.CostGuard.MaxCostPerDay > 0 {
		costGuard.SetLimits(CostLimits{
			PerTurn:        config.CostGuard.MaxTokensPerTurn,
			PerSession:     config.CostGuard.MaxTokensPerSession,
			PerDay:         config.CostGuard.MaxTokensPerDay,
			CostPerDay:     config.CostGuard.MaxCostPerDay,
			DeepPerSession: config.CostGuard.MaxDeepPerSession,
		})
	}
	return costGuard, 2, nil
}

func buildProvider(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	provURL := config.Provider.URL
	if provURL == "" {
		provURL = "http://localhost:12101"
	}
	provName := config.Provider.Type
	if provName == "" {
		provName = "provider"
	}
	if provName == "echo" {
		return &EchoProvider{}, 100, nil
	}
	return NewHTTPProvider(provName, provURL, config.Provider.Model), 100, nil
}

// --- Optional Builders ---

func buildReasoning(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	addon := NewReasoningAddon(defaultMaxBacktracks)
	addon.SetDispatcher(dispatcher)
	return addon, 10, nil
}

func buildMemoryComposer(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	memComposer := NewMemoryComposerAddon(defaultMemoryBudget)
	factsLayer := NewFactsLayer()
	memComposer.Composer().Register(factsLayer)
	memComposer.Composer().Register(NewRecentMessagesLayer(defaultRecentMessages))
	memComposer.Composer().Register(NewSessionSummaryLayer(defaultSummaryTokens))
	memComposer.Composer().Register(&ToolsLayer{})
	return memComposer, 15, nil
}

func buildCompaction(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	// Compaction needs FactsLayer — get it from memory_composer if available
	// The wiring happens post-build in main.go
	factsLayer := NewFactsLayer()
	summaryLayer := NewSessionSummaryLayer(defaultSummaryTokens)
	return NewCompactionAddon(factsLayer, summaryLayer), 16, nil
}

func buildFileUpload(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewFileUploadAddon(defaultMaxUploadMB), 7, nil
}

func buildWebSearch(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	switch config.WebSearch.Engine {
	case "searxng":
		return NewWebSearchAddonWithSearXNG(config.WebSearch.URL), 50, nil
	default:
		return NewWebSearchAddon(), 50, nil
	}
}

func buildChatLog(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewChatLogAddon(config.Logging.LogDir), 90, nil
}

func buildTranscript(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewTranscriptAddon(config.Logging.LogDir), 91, nil
}

func buildMCPManager(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	home, _ := os.UserHomeDir()
	return NewMCPManager(dispatcher, filepath.Join(home, ".neo-heinzel", "permissions.yaml")), 20, nil
}

func buildHeartbeat(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	if config.Heartbeat.File == "" {
		// No heartbeat file configured — return disabled addon
		return core.NewHeartbeatAddon(core.HeartbeatConfig{}), 95, nil
	}
	interval := core.DefaultHeartbeatInterval
	if config.Heartbeat.Interval != "" {
		if dur, err := time.ParseDuration(config.Heartbeat.Interval); err == nil {
			interval = dur
		}
	}
	return core.NewHeartbeatAddon(core.HeartbeatConfig{
		Interval: interval,
		FilePath: config.Heartbeat.File,
	}), 95, nil
}

func buildCognitiveMemory(config *core.Config, dispatcher *core.Dispatcher) (core.Addon, int, error) {
	return NewMemoryAddon(MemoryConfig{
		LLMEndpoint: config.Memory.LLMEndpoint,
		LLMModel:    config.Memory.LLMModel,
		PrologURL:   config.Memory.PrologURL,
		VaultURL:    config.Memory.VaultURL,
		ScriptURL:   config.Memory.ScriptURL,
	}), 8, nil // priority 8: after fileupload (7), before reasoning (10)
}
