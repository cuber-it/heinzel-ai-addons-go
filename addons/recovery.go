// RecoveryAddon — error handling, circuit breaker, retry classification.

package addons

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// RecoveryAction defines what to do on error
type RecoveryAction int

const (
	RecoveryRetry    RecoveryAction = iota // try again (maybe with different params)
	RecoveryFallback                       // switch to alternative
	RecoveryGraceful                       // inform user, continue without feature
	RecoveryShutdown                       // clean shutdown
)

// RecoveryAddon handles errors gracefully across all hooks
// Catches panics, retries transient errors, falls back when possible
type RecoveryAddon struct {
	core.BaseAddon
	mu            sync.Mutex
	errorCounts   map[string]int       // source → consecutive error count
	lastErrors    map[string]time.Time // source → last error time
	maxRetries    int
	circuitBreaker map[string]time.Time // source → open until (circuit breaker)
}

func NewRecoveryAddon() *RecoveryAddon {
	return &RecoveryAddon{
		errorCounts:    make(map[string]int),
		lastErrors:     make(map[string]time.Time),
		circuitBreaker: make(map[string]time.Time),
		maxRetries:     2,
	}
}

func (addon *RecoveryAddon) Name() string           { return "recovery" }
func (addon *RecoveryAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *RecoveryAddon) Start() error            { return nil }
func (addon *RecoveryAddon) Stop() error             { return nil }

func (addon *RecoveryAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnLLMError,
		core.OnToolError,
		core.OnLLMFallback,
	}
}

func (addon *RecoveryAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "health", Description: "system health status", Usage: "health"},
	}
}

func (addon *RecoveryAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	var lines []string
	lines = append(lines, "System Health:")

	// Provider
	providerStatus := "ok"
	if count, ok := addon.errorCounts["provider"]; ok && count > 0 {
		providerStatus = fmt.Sprintf("errors: %d", count)
	}
	if until, ok := addon.circuitBreaker["provider"]; ok && time.Now().Before(until) {
		providerStatus = fmt.Sprintf("circuit open (until %s)", until.Format("15:04:05"))
	}
	lines = append(lines, fmt.Sprintf("  Provider:    %s", providerStatus))

	// MCP
	mcpStatus := "ok"
	for source, count := range addon.errorCounts {
		if strings.HasPrefix(source, "mcp:") && count > 0 {
			mcpStatus = fmt.Sprintf("%s errors: %d", source, count)
		}
	}
	lines = append(lines, fmt.Sprintf("  MCP Tools:   %s", mcpStatus))

	// Web Search
	webStatus := "ok"
	if count, ok := addon.errorCounts["websearch"]; ok && count > 0 {
		webStatus = fmt.Sprintf("errors: %d", count)
	}
	lines = append(lines, fmt.Sprintf("  Web Search:  %s", webStatus))

	return strings.Join(lines, "\n")
}

func (addon *RecoveryAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnLLMError:
		return addon.handleLLMError(ctx)
	case core.OnToolError:
		return addon.handleToolError(ctx)
	case core.OnLLMFallback:
		return addon.handleFallback(ctx)
	}
	return core.Result{}
}

func (addon *RecoveryAddon) handleLLMError(ctx *core.Context) core.Result {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	addon.errorCounts["provider"]++
	addon.lastErrors["provider"] = time.Now()
	count := addon.errorCounts["provider"]

	thinking := core.GetThinking(ctx)
	errMsg := ""
	if ctx.Error != nil {
		errMsg = ctx.Error.Error()
	}

	if count <= addon.maxRetries && isTransient(errMsg) {
		if thinking != nil {
			thinking.AddStep("validate", fmt.Sprintf("provider error (attempt %d/%d), retrying...",
				count, addon.maxRetries), "recovery")
		}
		ctx.Error = nil
		ctx.Set(KeyNeedsRerun, true)
		return core.Result{}
	}

	if count > addon.maxRetries*2 {
		addon.circuitBreaker["provider"] = time.Now().Add(60 * time.Second)
		if thinking != nil {
			thinking.AddStep("validate", "provider circuit breaker open (60s cooldown)", "recovery")
		}
		ctx.Output = "Provider vorübergehend nicht erreichbar. Bitte in einer Minute erneut versuchen."
		ctx.Halt = true
		return core.Result{Halt: true}
	}

	if thinking != nil {
		thinking.AddStep("validate", fmt.Sprintf("provider error: %s", errMsg), "recovery")
	}
	ctx.Output = fmt.Sprintf("Fehler beim LLM-Aufruf: %s\nVersuche es erneut oder wechsle den Provider mit /provider.", errMsg)
	ctx.Halt = true
	return core.Result{Halt: true}
}

func (addon *RecoveryAddon) handleToolError(ctx *core.Context) core.Result {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	source := "mcp:unknown"
	if len(ctx.ToolCalls) > 0 {
		source = "mcp:" + ctx.ToolCalls[0].Name
	}
	addon.errorCounts[source]++

	thinking := core.GetThinking(ctx)

	if thinking != nil {
		errMsg := ""
		if ctx.Error != nil {
			errMsg = ctx.Error.Error()
		}
		thinking.AddStep("validate", fmt.Sprintf("tool error: %s — continuing without tool", errMsg), "recovery")
	}

	ctx.Error = nil
	return core.Result{}
}

func (addon *RecoveryAddon) handleFallback(ctx *core.Context) core.Result {
	thinking := core.GetThinking(ctx)
	if thinking != nil {
		thinking.AddStep("validate", "attempting provider fallback", "recovery")
	}

	addon.mu.Lock()
	addon.errorCounts["provider"] = 0
	addon.mu.Unlock()

	return core.Result{}
}

func (addon *RecoveryAddon) ResetErrors(source string) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	delete(addon.errorCounts, source)
	delete(addon.circuitBreaker, source)
}

func isTransient(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	transientPatterns := []string{
		"timeout", "deadline exceeded",
		"connection refused", "connection reset",
		"429", "rate limit", "too many requests",
		"500", "502", "503", "504",
		"internal server error", "bad gateway", "service unavailable",
		"temporary", "try again",
	}
	for _, pattern := range transientPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
