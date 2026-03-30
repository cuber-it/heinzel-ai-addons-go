package addons

import (
	"os"
	"testing"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewCostGuardAddon(test *testing.T) {
	addon := NewCostGuardAddon()
	if addon == nil {
		test.Fatal("NewCostGuardAddon returned nil")
	}
	if addon.Name() != "costguard" {
		test.Errorf("expected name 'costguard', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
	hooks := addon.Hooks()
	if len(hooks) != 3 {
		test.Errorf("expected 3 hooks, got %d", len(hooks))
	}
}

func TestCostGuardSetLimits(test *testing.T) {
	addon := NewCostGuardAddon()
	limits := CostLimits{
		PerTurn:        8192,
		PerSession:     100000,
		PerDay:         500000,
		CostPerDay:     10.0,
		DeepPerSession: 5,
	}
	addon.SetLimits(limits)

	if addon.maxTokensPerTurn != 8192 {
		test.Errorf("expected maxTokensPerTurn 8192, got %d", addon.maxTokensPerTurn)
	}
	if addon.maxTokensPerSession != 100000 {
		test.Errorf("expected maxTokensPerSession 100000, got %d", addon.maxTokensPerSession)
	}
	if addon.maxTokensPerDay != 500000 {
		test.Errorf("expected maxTokensPerDay 500000, got %d", addon.maxTokensPerDay)
	}
	if addon.maxCostPerDay != 10.0 {
		test.Errorf("expected maxCostPerDay 10.0, got %f", addon.maxCostPerDay)
	}
	if addon.maxDeepPerSession != 5 {
		test.Errorf("expected maxDeepPerSession 5, got %d", addon.maxDeepPerSession)
	}
}

func TestCostGuardSetLimitsZeroIgnored(test *testing.T) {
	addon := NewCostGuardAddon()
	originalTurn := addon.maxTokensPerTurn
	addon.SetLimits(CostLimits{PerTurn: 0})
	if addon.maxTokensPerTurn != originalTurn {
		test.Errorf("zero value should not change limit; expected %d, got %d", originalTurn, addon.maxTokensPerTurn)
	}
}

func TestCostGuardBudgetStatusInitial(test *testing.T) {
	addon := NewCostGuardAddon()
	pct, total, limit := addon.BudgetStatus()
	if pct != 0 {
		test.Errorf("expected 0%% initially, got %.2f%%", pct)
	}
	if total != 0 {
		test.Errorf("expected total 0, got %d", total)
	}
	if limit != defaultMaxTokensPerSession {
		test.Errorf("expected limit %d, got %d", defaultMaxTokensPerSession, limit)
	}
}

func TestCostGuardOnLLMCallBlocksWhenOverSessionBudget(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	// Artificially set counters to exceed session budget
	addon.mu.Lock()
	counters := addon.getOrCreateSession(ctx.SessionID)
	counters.tokensIn = 300000
	counters.tokensOut = 200001
	addon.mu.Unlock()

	result := addon.Handle(core.OnLLMCall, ctx)

	if !result.Halt {
		test.Error("expected Halt when session budget exceeded")
	}
	if !ctx.Halt {
		test.Error("expected ctx.Halt to be true")
	}
	if ctx.Error == nil {
		test.Error("expected error about budget exhaustion")
	}
}

func TestCostGuardOnLLMCallBlocksWhenOverDayBudget(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	addon.mu.Lock()
	addon.dayTokensIn = 1000000
	addon.dayTokensOut = 1000001
	addon.mu.Unlock()

	result := addon.Handle(core.OnLLMCall, ctx)

	if !result.Halt {
		test.Error("expected Halt when daily token budget exceeded")
	}
}

func TestCostGuardOnLLMCallBlocksWhenOverCostBudget(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	addon.SetLimits(CostLimits{CostPerDay: 1.0})
	ctx := testContext()

	// Set enough day tokens to exceed $1.00 at default pricing ($0.40/1M in, $1.60/1M out)
	addon.mu.Lock()
	addon.dayTokensIn = 1000000
	addon.dayTokensOut = 1000000
	addon.mu.Unlock()

	result := addon.Handle(core.OnLLMCall, ctx)

	if !result.Halt {
		test.Error("expected Halt when daily cost budget exceeded")
	}
}

func TestCostGuardOnLLMResponseCountsTokens(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	// Set token counts via context keys (simulating provider reporting)
	ctx.Set(KeyLLMTokensIn, 500)
	ctx.Set(KeyLLMTokensOut, 200)

	addon.Handle(core.OnLLMResponse, ctx)

	addon.mu.Lock()
	counters := addon.getOrCreateSession(ctx.SessionID)
	sessionIn := counters.tokensIn
	sessionOut := counters.tokensOut
	dayIn := addon.dayTokensIn
	dayOut := addon.dayTokensOut
	addon.mu.Unlock()

	if sessionIn != 500 {
		test.Errorf("expected sessionTokensIn 500, got %d", sessionIn)
	}
	if sessionOut != 200 {
		test.Errorf("expected sessionTokensOut 200, got %d", sessionOut)
	}
	if dayIn != 500 {
		test.Errorf("expected dayTokensIn 500, got %d", dayIn)
	}
	if dayOut != 200 {
		test.Errorf("expected dayTokensOut 200, got %d", dayOut)
	}

	// Check that turn cost was set on context
	turnCost, ok := ctx.Get(KeyTurnCost)
	if !ok {
		test.Error("expected KeyTurnCost to be set")
	}
	if cost, ok := turnCost.(float64); !ok || cost <= 0 {
		test.Errorf("expected positive turn cost, got %v", turnCost)
	}
}

func TestCostGuardSecretPassphraseOverride(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	// Default secret is "sesam"
	result := addon.HandleCommand("costs", "raise sesam session 999999", ctx)
	if result != "Session-Budget auf 999999 Tokens gesetzt." {
		test.Errorf("unexpected result: %s", result)
	}
	if addon.maxTokensPerSession != 999999 {
		test.Errorf("expected maxTokensPerSession 999999, got %d", addon.maxTokensPerSession)
	}
}

func TestCostGuardSecretPassphraseWrongSecret(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	result := addon.HandleCommand("costs", "raise wrong session 999999", ctx)
	if result != "Nope." {
		test.Errorf("expected 'Nope.' for wrong secret, got %q", result)
	}
}

func TestCostGuardSecretFromEnv(test *testing.T) {
	os.Setenv("HEINZEL_COST_SECRET", "testsecret42")
	defer os.Unsetenv("HEINZEL_COST_SECRET")

	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	result := addon.HandleCommand("costs", "raise testsecret42 day 20", ctx)
	if addon.maxCostPerDay != 20.0 {
		test.Errorf("expected maxCostPerDay 20, got %f (result: %s)", addon.maxCostPerDay, result)
	}
}

func TestCostGuardDayResetOnSessionStart(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	// Set counters and fake the day start to >24h ago
	addon.mu.Lock()
	addon.dayTokensIn = 50000
	addon.dayTokensOut = 30000
	addon.dayStart = time.Now().Add(-25 * time.Hour)
	addon.mu.Unlock()

	addon.Handle(core.OnSessionStart, ctx)

	addon.mu.Lock()
	dayIn := addon.dayTokensIn
	dayOut := addon.dayTokensOut
	addon.mu.Unlock()

	if dayIn != 0 {
		test.Errorf("expected dayTokensIn reset to 0, got %d", dayIn)
	}
	if dayOut != 0 {
		test.Errorf("expected dayTokensOut reset to 0, got %d", dayOut)
	}
}

func TestCostGuardSessionStartResetsSessionCounters(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	ctx := testContext()

	addon.mu.Lock()
	counters := addon.getOrCreateSession(ctx.SessionID)
	counters.tokensIn = 10000
	counters.tokensOut = 5000
	counters.deepCount = 3
	addon.mu.Unlock()

	addon.Handle(core.OnSessionStart, ctx)

	addon.mu.Lock()
	counters = addon.getOrCreateSession(ctx.SessionID)
	sessionIn := counters.tokensIn
	sessionOut := counters.tokensOut
	deep := counters.deepCount
	addon.mu.Unlock()

	if sessionIn != 0 || sessionOut != 0 || deep != 0 {
		test.Errorf("expected session counters reset, got in=%d out=%d deep=%d", sessionIn, sessionOut, deep)
	}
}

func TestCostGuardDeepReasoningLimit(test *testing.T) {
	addon := NewCostGuardAddon()
	addon.Start()
	addon.SetLimits(CostLimits{DeepPerSession: 2})
	ctx := testContext()

	// Set strategy to deep reasoning
	ctx.Set(KeyStrategy, StrategyDeepReasoning)

	// First call should increment deep count
	addon.Handle(core.OnLLMCall, ctx)
	addon.mu.Lock()
	counters := addon.getOrCreateSession(ctx.SessionID)
	deep := counters.deepCount
	addon.mu.Unlock()
	if deep != 1 {
		test.Errorf("expected deepCount 1, got %d", deep)
	}

	// Second call
	ctx.Set(KeyStrategy, StrategyDeepReasoning)
	addon.Handle(core.OnLLMCall, ctx)
	addon.mu.Lock()
	deep = addon.getOrCreateSession(ctx.SessionID).deepCount
	addon.mu.Unlock()
	if deep != 2 {
		test.Errorf("expected deepCount 2, got %d", deep)
	}

	// Third call should downgrade to CoT
	ctx.Set(KeyStrategy, StrategyDeepReasoning)
	addon.Handle(core.OnLLMCall, ctx)

	strategy, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if strategy != StrategyChainOfThought {
		test.Errorf("expected strategy downgraded to CoT, got %v", strategy)
	}
}

func TestPricingForModel(test *testing.T) {
	inputPrice, outputPrice := PricingForModel("gpt-4.1-mini")
	if inputPrice != 0.40 || outputPrice != 1.60 {
		test.Errorf("gpt-4.1-mini pricing wrong: in=%.2f out=%.2f", inputPrice, outputPrice)
	}

	inputPrice, outputPrice = PricingForModel("claude-sonnet-4")
	if inputPrice != 3.00 || outputPrice != 15.00 {
		test.Errorf("sonnet pricing wrong: in=%.2f out=%.2f", inputPrice, outputPrice)
	}

	inputPrice, outputPrice = PricingForModel("unknown-model")
	if inputPrice != 1.00 || outputPrice != 4.00 {
		test.Errorf("default pricing wrong: in=%.2f out=%.2f", inputPrice, outputPrice)
	}
}
