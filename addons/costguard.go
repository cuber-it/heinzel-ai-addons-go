// CostGuardAddon — token budget enforcement and cost tracking.

package addons

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const (
	defaultMaxTokensPerTurn    = 4096
	defaultMaxTokensPerSession = 500_000
	defaultMaxTokensPerDay     = 2_000_000
	defaultMaxCostPerDay       = 5.00
	defaultMaxDeepPerSession   = 10
	defaultInputCostPerM       = 0.40
	defaultOutputCostPerM      = 1.60
)

// Token costs per 1M tokens (approximate, as of March 2026)
// Source: public pricing pages
//
// OpenAI:
//   gpt-4.1-mini:  input $0.40/1M, output $1.60/1M
//   gpt-4.1:       input $2.00/1M, output $8.00/1M
//   gpt-4o:        input $2.50/1M, output $10.00/1M
//   o3-mini:       input $1.10/1M, output $4.40/1M
//
// Anthropic:
//   claude-sonnet:  input $3.00/1M, output $15.00/1M
//   claude-haiku:   input $0.80/1M, output $4.00/1M
//   claude-opus:    input $15.00/1M, output $75.00/1M
//
// Claude Code (Anthropic Max plan): $100/month flat for Sonnet, $200 for Opus
// ChatGPT Plus: $20/month flat (limited usage, then throttled)
// ChatGPT Pro: $200/month flat (unlimited)

type sessionCounters struct {
	tokensIn  int
	tokensOut int
	deepCount int
}

// CostGuardAddon prevents runaway costs
// Budget can ONLY be raised by the user via a secret passphrase — never by the agent
type CostGuardAddon struct {
	core.BaseAddon
	mu sync.Mutex

	// Limits (configurable)
	maxTokensPerTurn    int     // hard limit per LLM response
	maxTokensPerSession int     // total budget per session
	maxTokensPerDay     int     // daily limit across all sessions
	maxCostPerDay       float64 // in dollars
	maxDeepPerSession   int     // max deep reasoning turns per session

	// Per-session counters (sessionID → counters)
	sessions map[string]*sessionCounters

	// Global daily counters (correct — day budget spans all sessions)
	dayTokensIn  int
	dayTokensOut int
	dayStart     time.Time
	turnStart    time.Time

	// Pricing (per 1M tokens)
	inputCostPerM  float64
	outputCostPerM float64

	// Secret passphrase for budget override — from env, NEVER in prompts or context
	overrideSecret string
}

func NewCostGuardAddon() *CostGuardAddon {
	return &CostGuardAddon{
		// Conservative defaults
		maxTokensPerTurn:    defaultMaxTokensPerTurn,
		maxTokensPerSession: defaultMaxTokensPerSession,
		maxTokensPerDay:     defaultMaxTokensPerDay,
		maxCostPerDay:       defaultMaxCostPerDay,
		maxDeepPerSession:   defaultMaxDeepPerSession,

		// Per-session counters
		sessions: make(map[string]*sessionCounters),

		// Default pricing: gpt-4.1-mini
		inputCostPerM:  defaultInputCostPerM,
		outputCostPerM: defaultOutputCostPerM,
		dayStart:       time.Now(),
	}
}

func (addon *CostGuardAddon) BudgetStatus() (usedPct float64, total int, limit int) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	for _, counters := range addon.sessions {
		total += counters.tokensIn + counters.tokensOut
	}
	limit = addon.maxTokensPerSession
	if limit > 0 {
		usedPct = float64(total) / float64(limit) * 100
	}
	return
}

func (addon *CostGuardAddon) BudgetStatusForSession(sessionID string) (usedPct float64, total int, limit int) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	limit = addon.maxTokensPerSession
	if counters, ok := addon.sessions[sessionID]; ok {
		total = counters.tokensIn + counters.tokensOut
	}
	if limit > 0 {
		usedPct = float64(total) / float64(limit) * 100
	}
	return
}

func (addon *CostGuardAddon) Name() string           { return "costguard" }
func (addon *CostGuardAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *CostGuardAddon) Start() error {
	addon.overrideSecret = os.Getenv("HEINZEL_COST_SECRET")
	if addon.overrideSecret == "" {
		addon.overrideSecret = "sesam" // default, change via env
	}
	return nil
}
func (addon *CostGuardAddon) Stop() error             { return nil }

func (addon *CostGuardAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnLLMCall,     // check budget before call
		core.OnLLMResponse, // count tokens after call
		core.OnSessionStart,
	}
}

func (addon *CostGuardAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "costs", Description: "show token usage and costs",
			Usage: "/costs [status|limits|reset]"},
	}
}

func (addon *CostGuardAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = parts[0]
	}

	switch sub {
	case "", "status":
		return addon.formatStatus(ctx.SessionID)

	case "limits":
		return fmt.Sprintf(
			"Per turn:    %d tokens\n"+
				"Per session: %d tokens\n"+
				"Per day:     %d tokens\n"+
				"Cost/day:    $%.2f\n"+
				"Deep/session: %d\n"+
				"Pricing:     $%.2f/1M in, $%.2f/1M out",
			addon.maxTokensPerTurn, addon.maxTokensPerSession,
			addon.maxTokensPerDay, addon.maxCostPerDay,
			addon.maxDeepPerSession,
			addon.inputCostPerM, addon.outputCostPerM)

	case "reset":
		addon.resetSession(ctx.SessionID)
		return "Session counters reset."

	case "raise":
	// Secret passphrase required — ONLY the user can do this
		if len(parts) < 4 {
			return "Nope."
		}
		secret := parts[1]
		if secret != addon.overrideSecret {
			return "Nope."
		}
		field := parts[2]
		var value float64
		fmt.Sscanf(parts[3], "%f", &value)

		switch field {
		case "day":
			addon.maxCostPerDay = value
			return fmt.Sprintf("Tagesbudget auf $%.2f gesetzt.", value)
		case "session":
			addon.maxTokensPerSession = int(value)
			return fmt.Sprintf("Session-Budget auf %d Tokens gesetzt.", int(value))
		case "deep":
			addon.maxDeepPerSession = int(value)
			return fmt.Sprintf("Deep-Limit auf %d gesetzt.", int(value))
		case "turn":
			addon.maxTokensPerTurn = int(value)
			return fmt.Sprintf("Turn-Limit auf %d Tokens gesetzt.", int(value))
		default:
			return "Felder: day, session, deep, turn"
		}
	}
	return "Usage: /costs [status|limits|reset]"
}

func (addon *CostGuardAddon) formatStatus(sessionID string) string {
	counters := addon.getOrCreateSession(sessionID)
	sessionTotal := counters.tokensIn + counters.tokensOut
	dayTotal := addon.dayTokensIn + addon.dayTokensOut
	dayCost := addon.estimateCost(addon.dayTokensIn, addon.dayTokensOut)

	sessionRemain := addon.maxTokensPerSession - sessionTotal
	if sessionRemain < 0 {
		sessionRemain = 0
	}
	dayRemain := addon.maxTokensPerDay - dayTotal
	if dayRemain < 0 {
		dayRemain = 0
	}
	costRemain := addon.maxCostPerDay - dayCost
	if costRemain < 0 {
		costRemain = 0
	}
	sessionPct := float64(sessionTotal) / float64(addon.maxTokensPerSession) * 100
	dayPct := float64(dayTotal) / float64(addon.maxTokensPerDay) * 100
	costPct := dayCost / addon.maxCostPerDay * 100

	sessionWarn := ""
	if sessionPct > 80 {
		sessionWarn = " ⚠"
	}
	if sessionPct > 95 {
		sessionWarn = " 🛑"
	}
	dayWarn := ""
	if dayPct > 80 {
		dayWarn = " ⚠"
	}
	costWarn := ""
	if costPct > 80 {
		costWarn = " ⚠"
	}

	return fmt.Sprintf(
		"Session: %d / %d tokens (%.0f%%)%s — %d remaining\n"+
			"Day:     %d / %d tokens (%.0f%%)%s — %d remaining\n"+
			"Cost:    $%.4f / $%.2f (%.0f%%)%s — $%.4f remaining\n"+
			"Deep:    %d / %d used",
		sessionTotal, addon.maxTokensPerSession, sessionPct, sessionWarn, sessionRemain,
		dayTotal, addon.maxTokensPerDay, dayPct, dayWarn, dayRemain,
		dayCost, addon.maxCostPerDay, costPct, costWarn, costRemain,
		counters.deepCount, addon.maxDeepPerSession)
}

// Must be called with addon.mu held.
func (addon *CostGuardAddon) getOrCreateSession(sessionID string) *sessionCounters {
	if sessionID == "" {
		sessionID = "_default"
	}
	counters, ok := addon.sessions[sessionID]
	if !ok {
		counters = &sessionCounters{}
		addon.sessions[sessionID] = counters
	}
	return counters
}

func (addon *CostGuardAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	switch hook {
	case core.OnSessionStart:
		addon.resetSession(ctx.SessionID)
	case core.OnLLMCall:
		return addon.checkBudgetBefore(ctx)
	case core.OnLLMResponse:
		addon.recordTokensAfter(ctx)
	}
	return core.Result{}
}

// Must be called with addon.mu held.
func (addon *CostGuardAddon) resetSession(sessionID string) {
	if sessionID == "" {
		sessionID = "_default"
	}
	addon.sessions[sessionID] = &sessionCounters{}
	if time.Since(addon.dayStart) > 24*time.Hour {
		addon.dayTokensIn = 0
		addon.dayTokensOut = 0
		addon.dayStart = time.Now()
	}
}

// Must be called with addon.mu held.
func (addon *CostGuardAddon) checkBudgetBefore(ctx *core.Context) core.Result {
	addon.turnStart = time.Now()
	counters := addon.getOrCreateSession(ctx.SessionID)

	// Internal queries (triage, reasoning) count but pass through
	if val, ok := ctx.Get(KeyInternalQuery); ok {
		if internal, ok := val.(bool); ok && internal {
			return core.Result{}
		}
	}

	totalSession := counters.tokensIn + counters.tokensOut
	if addon.maxTokensPerSession > 0 && totalSession >= addon.maxTokensPerSession {
		ctx.Error = fmt.Errorf("session budget exhausted (%d/%d tokens)", totalSession, addon.maxTokensPerSession)
		ctx.Halt = true
		return core.Result{Halt: true}
	}

	totalDay := addon.dayTokensIn + addon.dayTokensOut
	if addon.maxTokensPerDay > 0 && totalDay >= addon.maxTokensPerDay {
		ctx.Output = fmt.Sprintf("[System] Tages-Budget erschöpft (%d/%d Tokens).",
			totalDay, addon.maxTokensPerDay)
		ctx.Halt = true
		return core.Result{Halt: true}
	}

	dayCost := addon.estimateCost(addon.dayTokensIn, addon.dayTokensOut)
	if addon.maxCostPerDay > 0 && dayCost >= addon.maxCostPerDay {
		ctx.Output = fmt.Sprintf("[System] Tagesbudget erschöpft ($%.2f/$%.2f).",
			dayCost, addon.maxCostPerDay)
		ctx.Halt = true
		return core.Result{Halt: true}
	}

	if val, ok := ctx.Get(KeyStrategy); ok {
		if strategy, ok := val.(Strategy); ok && strategy == StrategyDeepReasoning {
			if addon.maxDeepPerSession > 0 && counters.deepCount >= addon.maxDeepPerSession {
				ctx.Set(KeyStrategy, StrategyChainOfThought)
				thinking := core.GetThinking(ctx)
				if thinking != nil {
					thinking.AddStep("validate",
						fmt.Sprintf("deep reasoning limit reached (%d/%d), downgrading to CoT",
							counters.deepCount, addon.maxDeepPerSession), "costguard")
				}
			} else {
				counters.deepCount++
			}
		}
	}

	ctx.Set(KeyMaxOutputTokens, addon.maxTokensPerTurn)

	return core.Result{}
}

// Must be called with addon.mu held.
func (addon *CostGuardAddon) recordTokensAfter(ctx *core.Context) {
	counters := addon.getOrCreateSession(ctx.SessionID)

	inputTokens := 0
	outputTokens := 0

	if val, ok := ctx.Get(KeyLLMTokensIn); ok {
		if n, ok := val.(int); ok {
			inputTokens = n
		}
	}
	if val, ok := ctx.Get(KeyLLMTokensOut); ok {
		if n, ok := val.(int); ok {
			outputTokens = n
		}
	}

	if inputTokens == 0 {
		inputTokens = ctx.TokenEstimate()
	}
	if outputTokens == 0 {
		outputTokens = len(ctx.Output) / 4
	}

	counters.tokensIn += inputTokens
	counters.tokensOut += outputTokens

	addon.dayTokensIn += inputTokens
	addon.dayTokensOut += outputTokens

	ctx.Set(KeyTurnTokensIn, inputTokens)
	ctx.Set(KeyTurnTokensOut, outputTokens)
	ctx.Set(KeyTurnCost, addon.estimateCost(inputTokens, outputTokens))

	cost := addon.estimateCost(inputTokens, outputTokens)
	ctx.Log.LogWithMeta("system", fmt.Sprintf("tokens: %d in + %d out (~$%.4f)",
		inputTokens, outputTokens, cost), "costguard", "on_llm_response", nil)
}

func (addon *CostGuardAddon) estimateCost(tokensIn, tokensOut int) float64 {
	inCost := float64(tokensIn) / 1000000.0 * addon.inputCostPerM
	outCost := float64(tokensOut) / 1000000.0 * addon.outputCostPerM
	return inCost + outCost
}

type CostLimits struct {
	PerTurn        int     // max tokens per LLM response
	PerSession     int     // total token budget per session
	PerDay         int     // daily token limit across all sessions
	CostPerDay     float64 // max cost per day in dollars
	DeepPerSession int     // max deep reasoning turns per session
}

func (addon *CostGuardAddon) SetLimits(limits CostLimits) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	if limits.PerTurn > 0 {
		addon.maxTokensPerTurn = limits.PerTurn
	}
	if limits.PerSession > 0 {
		addon.maxTokensPerSession = limits.PerSession
	}
	if limits.PerDay > 0 {
		addon.maxTokensPerDay = limits.PerDay
	}
	if limits.CostPerDay > 0 {
		addon.maxCostPerDay = limits.CostPerDay
	}
	if limits.DeepPerSession > 0 {
		addon.maxDeepPerSession = limits.DeepPerSession
	}
}

func (addon *CostGuardAddon) SetPricing(inputPerM, outputPerM float64) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	addon.inputCostPerM = inputPerM
	addon.outputCostPerM = outputPerM
}

func PricingForModel(model string) (inputPerM, outputPerM float64) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gpt-4.1-mini"):
		return 0.40, 1.60
	case strings.Contains(m, "gpt-4.1"):
		return 2.00, 8.00
	case strings.Contains(m, "gpt-4o-mini"):
		return 0.15, 0.60
	case strings.Contains(m, "gpt-4o"):
		return 2.50, 10.00
	case strings.Contains(m, "o3-mini"), strings.Contains(m, "o4-mini"):
		return 1.10, 4.40
	case strings.Contains(m, "haiku"):
		return 0.80, 4.00
	case strings.Contains(m, "sonnet"):
		return 3.00, 15.00
	case strings.Contains(m, "opus"):
		return 15.00, 75.00
	default:
		return 1.00, 4.00 // conservative default
	}
}
