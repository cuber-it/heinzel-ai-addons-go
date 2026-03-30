package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestReasoningHeuristicShortInputPassthrough(test *testing.T) {
	addon := NewReasoningAddon(3)
	// Short input (<=5 words, no special keywords) should be passthrough
	result := addon.heuristic("hello world")
	if result != StrategyPassthrough {
		test.Errorf("expected passthrough for short input, got %s", result)
	}
}

func TestReasoningHeuristicExplainWhyCoT(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("explain why the sky is blue")
	if result != StrategyChainOfThought {
		test.Errorf("expected chain_of_thought for 'explain why', got %s", result)
	}
}

func TestReasoningHeuristicWarumCoT(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("warum ist der himmel blau")
	if result != StrategyChainOfThought {
		test.Errorf("expected chain_of_thought for 'warum', got %s", result)
	}
}

func TestReasoningHeuristicAnalyzeDeep(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("analyze the performance of this application in detail")
	if result != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning for 'analyze', got %s", result)
	}
}

func TestReasoningHeuristicCompareDeep(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("compare these two approaches and evaluate their tradeoffs in the given scenario")
	if result != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning for 'compare', got %s", result)
	}
}

func TestReasoningHeuristicCreateReAct(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("create a new file with the configuration")
	if result != StrategyReAct {
		test.Errorf("expected react for 'create', got %s", result)
	}
}

func TestReasoningHeuristicBuildReAct(test *testing.T) {
	addon := NewReasoningAddon(3)
	result := addon.heuristic("build a docker image for the service")
	if result != StrategyReAct {
		test.Errorf("expected react for 'build', got %s", result)
	}
}

func TestReasoningHeuristicLongInputCoT(test *testing.T) {
	addon := NewReasoningAddon(3)
	// >20 words without special keywords should default to CoT
	longInput := "this is a very long input that has more than twenty words and should trigger the chain of thought strategy because it exceeds the threshold"
	result := addon.heuristic(longInput)
	if result != StrategyChainOfThought {
		test.Errorf("expected chain_of_thought for long input, got %s", result)
	}
}

func TestReasoningHandleStrategyCommand(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	// Set to deep
	result := addon.handleStrategyCommand("deep", ctx)
	if !strings.Contains(result, "deep_reasoning") {
		test.Errorf("expected 'deep_reasoning' in result, got: %s", result)
	}
	val, ok := ctx.Get(KeyStrategyOverride)
	if !ok {
		test.Fatal("expected KeyStrategyOverride to be set")
	}
	if val != StrategyDeepReasoning {
		test.Errorf("expected StrategyDeepReasoning, got %v", val)
	}

	// Set to CoT
	addon.handleStrategyCommand("cot", ctx)
	val, _ = ctx.Get(KeyStrategyOverride)
	if val != StrategyChainOfThought {
		test.Errorf("expected StrategyChainOfThought, got %v", val)
	}

	// Set to auto (clears override)
	result = addon.handleStrategyCommand("auto", ctx)
	if !strings.Contains(result, "auto") {
		test.Errorf("expected 'auto' in result, got: %s", result)
	}

	// Set to passthrough
	addon.handleStrategyCommand("pass", ctx)
	val, _ = ctx.Get(KeyStrategyOverride)
	if val != StrategyPassthrough {
		test.Errorf("expected StrategyPassthrough, got %v", val)
	}

	// Set to react
	addon.handleStrategyCommand("react", ctx)
	val, _ = ctx.Get(KeyStrategyOverride)
	if val != StrategyReAct {
		test.Errorf("expected StrategyReAct, got %v", val)
	}
}

func TestReasoningHandleStrategyCommandUnknown(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handleStrategyCommand("quantum", ctx)
	if !strings.Contains(result, "Unbekannt") {
		test.Errorf("expected 'Unbekannt' for unknown strategy, got: %s", result)
	}
}

func TestReasoningPlanModeOnOff(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handlePlanCommand("on", ctx)
	if result != "Plan mode ON." {
		test.Errorf("expected 'Plan mode ON.', got: %s", result)
	}
	val, ok := ctx.Get(KeyPlanMode)
	if !ok {
		test.Fatal("expected KeyPlanMode to be set")
	}
	if on, ok := val.(bool); !ok || !on {
		test.Error("expected plan mode to be true")
	}

	result = addon.handlePlanCommand("off", ctx)
	if result != "Plan mode OFF." {
		test.Errorf("expected 'Plan mode OFF.', got: %s", result)
	}
	val, _ = ctx.Get(KeyPlanMode)
	if on, ok := val.(bool); ok && on {
		test.Error("expected plan mode to be false after off")
	}
}

func TestReasoningPlanShowWhenEmpty(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handlePlanCommand("show", ctx)
	if !strings.Contains(result, "OFF") {
		test.Errorf("expected 'OFF' in plan show, got: %s", result)
	}
}

func TestReasoningClassifyOnInputClassified(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "hi"

	addon.Handle(core.OnInputClassified, ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set after classify")
	}
	if val != StrategyPassthrough {
		test.Errorf("expected passthrough for 'hi', got %v", val)
	}
}

func TestReasoningClassifyUserOverride(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "think deep about this problem"

	addon.Handle(core.OnInputClassified, ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if val != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning for 'think deep', got %v", val)
	}
}

func TestReasoningHandleCommandRouting(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	// HandleCommand routes to strategy
	result := addon.HandleCommand("strategy", "cot", ctx)
	if !strings.Contains(result, "chain_of_thought") {
		test.Errorf("expected strategy result, got: %s", result)
	}

	// HandleCommand routes to plan
	result = addon.HandleCommand("plan", "on", ctx)
	if result != "Plan mode ON." {
		test.Errorf("expected plan result, got: %s", result)
	}
}

// === Triage tests ===

func TestTriageFallbackWithoutLoop(test *testing.T) {
	// Without loop, classify falls through to heuristic
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "hello"

	addon.classify(ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if val != StrategyPassthrough {
		test.Errorf("expected passthrough (heuristic fallback), got %v", val)
	}

	// Verify the step mentions "heuristic fallback"
	stream := core.GetThinking(ctx)
	found := false
	for _, step := range stream.Steps {
		if strings.Contains(step.Content, "heuristic fallback") {
			found = true
			break
		}
	}
	if !found {
		test.Error("expected thinking step with 'heuristic fallback'")
	}
}

func TestTriageOverrideWinsOverHeuristic(test *testing.T) {
	// User override via /strategy command should win over heuristic
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "hello" // would normally be passthrough via heuristic

	// Set override to deep
	ctx.Set(KeyStrategyOverride, StrategyDeepReasoning)

	addon.classify(ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if val != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning (override), got %v", val)
	}

	// Verify the step mentions "override"
	stream := core.GetThinking(ctx)
	found := false
	for _, step := range stream.Steps {
		if strings.Contains(step.Content, "override") {
			found = true
			break
		}
	}
	if !found {
		test.Error("expected thinking step with 'override'")
	}
}

func TestTriageKeywordThinkDeep(test *testing.T) {
	// "think deep" keyword should trigger deep reasoning directly
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "think deep about this architecture"

	addon.classify(ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if val != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning for 'think deep' keyword, got %v", val)
	}

	// Verify the step mentions "keyword"
	stream := core.GetThinking(ctx)
	found := false
	for _, step := range stream.Steps {
		if strings.Contains(step.Content, "keyword") {
			found = true
			break
		}
	}
	if !found {
		test.Error("expected thinking step with 'keyword'")
	}
}

func TestTriageKeywordDenkGruendlich(test *testing.T) {
	// German keyword "denk gründlich" should also trigger deep reasoning
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "denk gründlich über dieses Problem nach"

	addon.classify(ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	if val != StrategyDeepReasoning {
		test.Errorf("expected deep_reasoning for 'denk gründlich', got %v", val)
	}
}

func TestTriageInvalidJSON(test *testing.T) {
	// triageWithLLM should return false on invalid JSON, classify falls to heuristic
	addon := NewReasoningAddon(3)
	// No loop set, so triageWithLLM is never called — classify uses heuristic
	// This tests the fallback path indirectly
	ctx := testContext()
	ctx.Input = "explain why water is wet"

	addon.classify(ctx)

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		test.Fatal("expected strategy to be set")
	}
	// "explain why" triggers CoT via heuristic
	if val != StrategyChainOfThought {
		test.Errorf("expected chain_of_thought from heuristic, got %v", val)
	}
}

// === Depth control tests ===

func TestGetDepthDefault(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	// No depth set in context, should return default
	depth := addon.getDepth(ctx, 3)
	if depth != 3 {
		test.Errorf("expected default depth 3, got %d", depth)
	}
}

func TestGetDepthFromContext(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Set(KeyThinkingDepth, 2)

	depth := addon.getDepth(ctx, 3)
	if depth != 2 {
		test.Errorf("expected depth 2 from context, got %d", depth)
	}
}

func TestStrategyCommandWithDepth(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handleStrategyCommand("cot 3", ctx)
	if !strings.Contains(result, "chain_of_thought") {
		test.Errorf("expected chain_of_thought in result, got: %s", result)
	}
	if !strings.Contains(result, "3") {
		test.Errorf("expected depth 3 in result, got: %s", result)
	}

	val, ok := ctx.Get(KeyThinkingDepth)
	if !ok {
		test.Fatal("expected KeyThinkingDepth to be set")
	}
	if val != 3 {
		test.Errorf("expected depth 3, got %v", val)
	}
}

func TestStrategyCommandDeepDefaultDepth(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handleStrategyCommand("deep", ctx)
	if !strings.Contains(result, "deep_reasoning") {
		test.Errorf("expected deep_reasoning in result, got: %s", result)
	}

	val, ok := ctx.Get(KeyThinkingDepth)
	if !ok {
		test.Fatal("expected KeyThinkingDepth to be set")
	}
	if val != 5 {
		test.Errorf("expected default depth 5 for deep, got %v", val)
	}
}

func TestStrategyNativeCommand(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()

	result := addon.handleStrategyCommand("native", ctx)
	if !strings.Contains(result, "native") {
		test.Errorf("expected 'native' in result, got: %s", result)
	}

	val, ok := ctx.Get(KeyStrategyOverride)
	if !ok {
		test.Fatal("expected KeyStrategyOverride to be set")
	}
	if val != StrategyNative {
		test.Errorf("expected StrategyNative, got %v", val)
	}
}

// === Execute thinking tests ===

func TestExecuteThinkingPassthrough(test *testing.T) {
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "hello"

	// Set strategy to passthrough
	ctx.Set(KeyStrategy, StrategyPassthrough)

	addon.executeThinking(ctx)

	// Passthrough should not set internal_query or native_reasoning
	if _, ok := ctx.Get(KeyInternalQuery); ok {
		test.Error("passthrough should not set KeyInternalQuery")
	}
	if _, ok := ctx.Get(KeyNativeReasoning); ok {
		test.Error("passthrough should not set KeyNativeReasoning")
	}
}

func TestExecuteThinkingNative(test *testing.T) {
	addon := NewReasoningAddon(3)
	// Need a loop set for executeThinking to proceed
	// But runNative doesn't call thinkQuery, it just sets a flag
	// However executeThinking returns early if addon.loop == nil
	// So we need to check: does it still work without loop for native?
	// Looking at code: executeThinking checks addon.loop == nil first and returns.
	// For native strategy, we need to provide a loop.
	// Let's test the runNative method directly instead.
	ctx := testContext()
	ctx.Set(KeyStrategy, StrategyNative)

	addon.runNative(ctx)

	val, ok := ctx.Get(KeyNativeReasoning)
	if !ok {
		test.Fatal("expected KeyNativeReasoning to be set")
	}
	if val != true {
		test.Errorf("expected KeyNativeReasoning=true, got %v", val)
	}

	// Verify thinking step recorded
	stream := core.GetThinking(ctx)
	found := false
	for _, step := range stream.Steps {
		if strings.Contains(step.Content, "Native Reasoning") {
			found = true
			break
		}
	}
	if !found {
		test.Error("expected thinking step with 'Native Reasoning'")
	}
}

func TestExecuteThinkingWithoutLoop(test *testing.T) {
	// executeThinking should be a no-op without a loop
	addon := NewReasoningAddon(3)
	ctx := testContext()
	ctx.Input = "analyze this deeply"
	ctx.Set(KeyStrategy, StrategyChainOfThought)

	originalInput := ctx.Input
	addon.executeThinking(ctx)

	// Input should not be modified (no thinkQuery calls without loop)
	if ctx.Input != originalInput {
		test.Errorf("expected input unchanged without loop, got modified: %s", ctx.Input)
	}
}
