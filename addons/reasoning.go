// ReasoningAddon — strategy selection, heuristic classification, multi-step thinking.
// Type aliases re-export core reasoning types for use within the addons package.

package addons

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

type Strategy = core.Strategy

const (
	StrategyPassthrough    = core.StrategyPassthrough
	StrategyChainOfThought = core.StrategyChainOfThought
	StrategyDeepReasoning  = core.StrategyDeepReasoning
	StrategyReAct          = core.StrategyReAct
	StrategyNative         = core.StrategyNative
)

// ReasoningAddon orchestrates strategy selection and multi-step reasoning.
type ReasoningAddon struct {
	core.BaseAddon
	loop           *core.Loop
	dispatcher     *core.Dispatcher
	maxBacktracks  int
	activePlan     *Plan
}

func NewReasoningAddon(maxBacktracks int) *ReasoningAddon {
	return &ReasoningAddon{
		maxBacktracks: maxBacktracks,
	}
}

func (addon *ReasoningAddon) Name() string           { return "reasoning" }
func (addon *ReasoningAddon) Type() core.AddonType   { return core.AddonFilter }
func (addon *ReasoningAddon) Start() error            { return nil }
func (addon *ReasoningAddon) Stop() error             { return nil }

func (addon *ReasoningAddon) SetDispatcher(dispatcher *core.Dispatcher) {
	addon.dispatcher = dispatcher
}

func (addon *ReasoningAddon) SetLoop(loop *core.Loop) {
	addon.loop = loop
}

func (addon *ReasoningAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnInputClassified,
		core.OnContextBuild,
		core.OnOutput,
	}
}

func (addon *ReasoningAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnInputClassified:
		addon.classify(ctx)
		addon.planIntercept(ctx)
		addon.executeThinking(ctx)
	case core.OnContextBuild:
		addon.injectPlanContext(ctx)
	case core.OnOutput:
		addon.parsePlanResponse(ctx)
	}
	return core.Result{}
}

func (addon *ReasoningAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "strategy", Description: "Set reasoning strategy", Usage: "strategy [auto|pass|cot|deep|react|native] [depth]"},
		{Name: "plan", Description: "Plan mode", Usage: "plan [on|off|show|approve|reject|skip|next|reset]"},
	}
}

func (addon *ReasoningAddon) HandleCommand(cmd string, args string, ctx *core.Context) string {
	switch cmd {
	case "strategy":
		return addon.handleStrategyCommand(args, ctx)
	case "plan":
		return addon.handlePlanCommand(args, ctx)
	}
	return ""
}

func (addon *ReasoningAddon) classify(ctx *core.Context) {
	stream := core.GetThinking(ctx)
	if stream == nil {
		stream = core.InitThinking(ctx, nil)
	}

	if val, ok := ctx.Get(KeyStrategyOverride); ok && val != nil {
		if strategy, ok := val.(Strategy); ok {
			ctx.Set(KeyStrategy, strategy)
			stream.AddStep("classify", fmt.Sprintf("override: %s", strategy), "reasoning")
			return
		}
	}

	input := strings.ToLower(ctx.Input)
	if strings.Contains(input, "think deep") || strings.Contains(input, "denk gründlich") {
		ctx.Set(KeyStrategy, StrategyDeepReasoning)
		stream.AddStep("classify", fmt.Sprintf("keyword: %s", StrategyDeepReasoning), "reasoning")
		return
	}

	words := strings.Fields(ctx.Input)
	if addon.loop != nil && len(words) > 5 {
		if strategy, ok := addon.triageWithLLM(ctx); ok {
			ctx.Set(KeyStrategy, strategy)
			stream.AddStep("classify", fmt.Sprintf("triage: %s", strategy), "reasoning")
			return
		}
	}

	strategy := addon.heuristic(ctx.Input)
	ctx.Set(KeyStrategy, strategy)
	stream.AddStep("classify", fmt.Sprintf("heuristic fallback: %s", strategy), "reasoning")
}

func (addon *ReasoningAddon) heuristic(input string) Strategy {
	lower := strings.ToLower(input)
	words := strings.Fields(lower)
	wordCount := len(words)

	if wordCount <= 5 {
		for _, word := range words {
			switch word {
			case "explain", "warum", "why", "wieso":
				return StrategyChainOfThought
			case "analyze", "analysiere", "compare", "vergleiche":
				return StrategyDeepReasoning
			case "create", "build", "erstelle", "baue":
				return StrategyReAct
			}
		}
		return StrategyPassthrough
	}

	for _, keyword := range []string{"analyze", "analysiere", "compare", "vergleiche", "evaluate", "bewerte"} {
		if strings.Contains(lower, keyword) {
			return StrategyDeepReasoning
		}
	}

	for _, keyword := range []string{"explain", "warum", "why", "wieso", "how does", "wie funktioniert"} {
		if strings.Contains(lower, keyword) {
			return StrategyChainOfThought
		}
	}

	for _, keyword := range []string{"create", "build", "erstelle", "baue", "deploy", "install", "run", "execute"} {
		if strings.Contains(lower, keyword) {
			return StrategyReAct
		}
	}

	if wordCount > 20 {
		return StrategyChainOfThought
	}

	return StrategyPassthrough
}

func (addon *ReasoningAddon) triageWithLLM(ctx *core.Context) (Strategy, bool) {
	if addon.loop == nil {
		return 0, false
	}

	triageCtx := core.NewContext("triage")
	triageCtx.SystemPrompt = `Classify the user's input into one of these strategies:
- passthrough: simple question or greeting
- cot: needs explanation or multi-step reasoning
- deep: needs thorough analysis
- react: needs tool use or action
- native: explicitly asks for model's deep thinking
Reply with ONLY the strategy name, nothing else.`
	triageCtx.Set(KeyInternalQuery, true)
	triageCtx.Set(KeyStrategyOverride, StrategyPassthrough)
	core.InitThinking(triageCtx, nil)

	result := strings.TrimSpace(strings.ToLower(addon.loop.Run(triageCtx, ctx.Input)))

	switch result {
	case "passthrough", "pass":
		return StrategyPassthrough, true
	case "cot", "chain_of_thought":
		return StrategyChainOfThought, true
	case "deep", "deep_reasoning":
		return StrategyDeepReasoning, true
	case "react":
		return StrategyReAct, true
	case "native":
		return StrategyNative, true
	}
	return 0, false
}

// handleStrategyCommand handles /strategy <args>
func (addon *ReasoningAddon) handleStrategyCommand(args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		val, _ := ctx.Get(KeyStrategyOverride)
		if val == nil {
			return "Strategy: auto"
		}
		return fmt.Sprintf("Strategy: %s", val)
	}

	name := strings.ToLower(parts[0])

	// Parse optional depth
	depth := 0
	if len(parts) > 1 {
		if parsed, err := strconv.Atoi(parts[1]); err == nil {
			depth = parsed
		}
	}

	var strategy Strategy
	switch name {
	case "auto":
		ctx.Set(KeyStrategyOverride, nil)
		return "Strategy: auto (heuristic/LLM triage)"
	case "pass", "passthrough":
		strategy = StrategyPassthrough
	case "cot", "chain_of_thought":
		strategy = StrategyChainOfThought
		if depth == 0 {
			depth = 3
		}
	case "deep", "deep_reasoning":
		strategy = StrategyDeepReasoning
		if depth == 0 {
			depth = 5
		}
	case "react":
		strategy = StrategyReAct
	case "native":
		strategy = StrategyNative
	default:
		return fmt.Sprintf("Unbekannt: %s. Optionen: auto, pass, cot, deep, react, native", name)
	}

	ctx.Set(KeyStrategyOverride, strategy)
	if depth > 0 {
		ctx.Set(KeyThinkingDepth, depth)
		return fmt.Sprintf("Strategy: %s (depth %d)", strategy, depth)
	}
	return fmt.Sprintf("Strategy: %s", strategy)
}
