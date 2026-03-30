package addons

import (
	"errors"
	"testing"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestRecoveryClassifiesTransientError(test *testing.T) {
	if !isTransient("connection timeout") {
		test.Error("expected 'connection timeout' to be transient")
	}
	if !isTransient("HTTP 429 Too Many Requests") {
		test.Error("expected '429' to be transient")
	}
	if !isTransient("503 Service Unavailable") {
		test.Error("expected '503' to be transient")
	}
	if !isTransient("rate limit exceeded") {
		test.Error("expected 'rate limit' to be transient")
	}
	if !isTransient("connection refused") {
		test.Error("expected 'connection refused' to be transient")
	}
	if !isTransient("please try again later") {
		test.Error("expected 'try again' to be transient")
	}
}

func TestRecoveryClassifiesNonTransientError(test *testing.T) {
	if isTransient("invalid api key") {
		test.Error("expected 'invalid api key' to be non-transient")
	}
	if isTransient("model not found") {
		test.Error("expected 'model not found' to be non-transient")
	}
	if isTransient("permission denied") {
		test.Error("expected 'permission denied' to be non-transient")
	}
	if isTransient("") {
		test.Error("expected empty string to be non-transient")
	}
}

func TestRecoveryRetriesTransientErrors(test *testing.T) {
	addon := NewRecoveryAddon()
	ctx := testContext()
	ctx.Error = errors.New("connection timeout")

	// First attempt: should retry (clear error, set needs_rerun)
	result := addon.Handle(core.OnLLMError, ctx)
	if result.Halt {
		test.Error("expected no halt on first transient error (should retry)")
	}
	if ctx.Error != nil {
		test.Error("expected error to be cleared for retry")
	}
	needsRerun, ok := ctx.Get(KeyNeedsRerun)
	if !ok {
		test.Fatal("expected KeyNeedsRerun to be set")
	}
	if rerun, ok := needsRerun.(bool); !ok || !rerun {
		test.Error("expected needs_rerun to be true")
	}
}

func TestRecoverySecondRetryStillWorks(test *testing.T) {
	addon := NewRecoveryAddon()

	// First retry
	ctx := testContext()
	ctx.Error = errors.New("connection timeout")
	addon.Handle(core.OnLLMError, ctx)

	// Second retry (maxRetries defaults to 2)
	ctx2 := testContext()
	ctx2.Error = errors.New("connection timeout again")
	result := addon.Handle(core.OnLLMError, ctx2)
	if result.Halt {
		test.Error("expected second retry to still work (maxRetries=2)")
	}
}

func TestRecoveryGracefulAfterMaxRetries(test *testing.T) {
	addon := NewRecoveryAddon()

	// Exhaust retries with transient errors
	for attempt := 0; attempt < 3; attempt++ {
		ctx := testContext()
		ctx.Error = errors.New("connection timeout")
		addon.Handle(core.OnLLMError, ctx)
	}

	// Next error should halt gracefully (count=3, > maxRetries=2, but <= maxRetries*2=4)
	ctx := testContext()
	ctx.Error = errors.New("connection timeout")
	result := addon.Handle(core.OnLLMError, ctx)
	if !result.Halt {
		test.Error("expected halt after exceeding max retries for transient error")
	}
}

func TestRecoveryCircuitBreakerTrips(test *testing.T) {
	addon := NewRecoveryAddon()

	// Trigger enough errors to trip the circuit breaker (count > maxRetries*2 = 4)
	for attempt := 0; attempt < 5; attempt++ {
		ctx := testContext()
		ctx.Error = errors.New("connection timeout")
		addon.Handle(core.OnLLMError, ctx)
	}

	// Check circuit breaker is open
	addon.mu.Lock()
	until, exists := addon.circuitBreaker["provider"]
	addon.mu.Unlock()

	if !exists {
		test.Fatal("expected circuit breaker to be set for provider")
	}
	if !time.Now().Before(until) {
		test.Error("expected circuit breaker to be in the future")
	}
}

func TestRecoveryCircuitBreakerResetsOnFallback(test *testing.T) {
	addon := NewRecoveryAddon()

	// Trigger errors to set error count
	for attempt := 0; attempt < 3; attempt++ {
		ctx := testContext()
		ctx.Error = errors.New("connection timeout")
		addon.Handle(core.OnLLMError, ctx)
	}

	// Simulate fallback which resets provider errors
	ctx := testContext()
	addon.Handle(core.OnLLMFallback, ctx)

	addon.mu.Lock()
	count := addon.errorCounts["provider"]
	addon.mu.Unlock()

	if count != 0 {
		test.Errorf("expected error count reset to 0 after fallback, got %d", count)
	}
}

func TestRecoveryResetErrors(test *testing.T) {
	addon := NewRecoveryAddon()

	addon.mu.Lock()
	addon.errorCounts["provider"] = 5
	addon.circuitBreaker["provider"] = time.Now().Add(60 * time.Second)
	addon.mu.Unlock()

	addon.ResetErrors("provider")

	addon.mu.Lock()
	_, hasCount := addon.errorCounts["provider"]
	_, hasBreaker := addon.circuitBreaker["provider"]
	addon.mu.Unlock()

	if hasCount {
		test.Error("expected error count to be deleted")
	}
	if hasBreaker {
		test.Error("expected circuit breaker to be deleted")
	}
}

func TestRecoveryToolErrorNeverHalts(test *testing.T) {
	addon := NewRecoveryAddon()
	ctx := testContext()
	ctx.Error = errors.New("tool failed")
	ctx.ToolCalls = []core.ToolCall{{Name: "test_tool"}}

	result := addon.Handle(core.OnToolError, ctx)

	if result.Halt {
		test.Error("tool errors should never halt")
	}
	if ctx.Error != nil {
		test.Error("tool error should be cleared")
	}
}

func TestRecoveryNonTransientErrorHalts(test *testing.T) {
	addon := NewRecoveryAddon()
	ctx := testContext()
	ctx.Error = errors.New("invalid api key")

	result := addon.Handle(core.OnLLMError, ctx)

	if !result.Halt {
		test.Error("expected halt on non-transient error")
	}
}

func TestRecoveryHealthCommand(test *testing.T) {
	addon := NewRecoveryAddon()
	ctx := testContext()

	result := addon.HandleCommand("health", "", ctx)
	if result == "" {
		test.Error("expected health output")
	}
	if !contains(result, "System Health") {
		test.Errorf("expected 'System Health' header, got: %s", result)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && searchString(haystack, needle)
}

func searchString(haystack, needle string) bool {
	for idx := 0; idx <= len(haystack)-len(needle); idx++ {
		if haystack[idx:idx+len(needle)] == needle {
			return true
		}
	}
	return false
}
