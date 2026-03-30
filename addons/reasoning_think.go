// Agent-level reasoning — the agent thinks, not the model.
// Multi-step LLM calls orchestrated by the addon, each step visible in ThinkingStream.

package addons

import (
	"fmt"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func (addon *ReasoningAddon) thinkQuery(systemPrompt, userMessage string) string {
	if addon.loop == nil {
		return ""
	}
	ctx := core.NewContext("reasoning")
	ctx.SystemPrompt = systemPrompt
	ctx.Set(KeyInternalQuery, true)
	ctx.Set(KeyStrategyOverride, StrategyPassthrough)
	core.InitThinking(ctx, nil)
	return addon.loop.Run(ctx, userMessage)
}

// runChainOfThought decomposes the question, analyzes aspects, synthesizes.
// Depth controls granularity: 1=decompose+synth, 2=+analyze, 3=+evaluate
func (addon *ReasoningAddon) runChainOfThought(ctx *core.Context, depth int) {
	stream := core.GetThinking(ctx)
	input := ctx.Input
	var aspects, analysis, evaluation string

	// Step 1: Decompose (depth >= 1)
	if stream != nil {
		stream.AddStep("reason", fmt.Sprintf("CoT depth %d — Zerlege die Frage...", depth), "reasoning")
	}
	aspects = addon.thinkQuery(
		"Du bist ein analytischer Denker. Zerlege die folgende Frage in 2-4 Kernaspekte die untersucht werden müssen. Antworte NUR mit einer nummerierten Liste, keine Erklärung.",
		input,
	)
	if stream != nil {
		stream.AddStep("reason", "Aspekte:\n"+aspects, "reasoning")
	}

	// Step 2: Analyze (depth >= 2)
	if depth >= 2 {
		if stream != nil {
			stream.AddStep("reason", "Analysiere Aspekte...", "reasoning")
		}
		analysis = addon.thinkQuery(
			"Du bist ein analytischer Denker. Analysiere die gegebenen Aspekte im Kontext der ursprünglichen Frage. Sei gründlich aber prägnant.",
			fmt.Sprintf("Frage: %s\n\nAspekte:\n%s", input, aspects),
		)
		if stream != nil {
			stream.AddStep("reason", "Analyse:\n"+analysis, "reasoning")
		}
	}

	// Step 3: Evaluate (depth >= 3)
	if depth >= 3 {
		if stream != nil {
			stream.AddStep("reason", "Prüfe kritisch...", "reasoning")
		}
		evalInput := fmt.Sprintf("Frage: %s\nAspekte:\n%s", input, aspects)
		if analysis != "" {
			evalInput += "\nAnalyse:\n" + analysis
		}
		evaluation = addon.thinkQuery(
			"Prüfe diese Analyse kritisch: Gibt es Lücken? Widersprüche? Fehlende Perspektiven? Antworte kurz.",
			evalInput,
		)
		if stream != nil {
			stream.AddStep("reason", "Bewertung:\n"+evaluation, "reasoning")
		}
	}

	// Synthesize — enrich the input for the final LLM call
	if stream != nil {
		stream.AddStep("reason", "Formuliere Antwort...", "reasoning")
	}

	var priorKnowledge strings.Builder
	priorKnowledge.WriteString("Aspekte:\n" + aspects)
	if analysis != "" {
		priorKnowledge.WriteString("\n\nAnalyse:\n" + analysis)
	}
	if evaluation != "" {
		priorKnowledge.WriteString("\n\nKritische Bewertung:\n" + evaluation)
	}

	// Inject analysis as turn prompt — don't overwrite ctx.Input to avoid message duplication
	ctx.Prompts.Set(core.LayerTurn, "reasoning_analysis",
		fmt.Sprintf("Voranalyse zur aktuellen Frage:\n%s\n\nNutze diese Voranalyse als Grundlage für deine Antwort.", priorKnowledge.String()), 85)
}

// runDeepReasoning does thorough multi-step analysis.
// Depth controls granularity: 1=understand+synth, 2=+decompose, 3=+analyze, 4=+evaluate, 5=+challenge
func (addon *ReasoningAddon) runDeepReasoning(ctx *core.Context, depth int) {
	stream := core.GetThinking(ctx)
	input := ctx.Input
	var understanding, aspects, analysis, evaluation, challenge string

	// Step 1: Understand (depth >= 1)
	if stream != nil {
		stream.AddStep("reason", fmt.Sprintf("Deep depth %d — Was wird gefragt?", depth), "reasoning")
	}
	understanding = addon.thinkQuery(
		"Beschreibe in 2-3 Sätzen was genau diese Frage/Aufgabe verlangt. Was ist die Kernfrage? Welche impliziten Annahmen stecken drin?",
		input,
	)
	if stream != nil {
		stream.AddStep("reason", "Verständnis:\n"+understanding, "reasoning")
	}

	// Step 2: Decompose (depth >= 2)
	if depth >= 2 {
		if stream != nil {
			stream.AddStep("reason", "Zerlege in Aspekte...", "reasoning")
		}
		aspects = addon.thinkQuery(
			"Zerlege diese Aufgabe in 3-5 Aspekte die analysiert werden müssen. NUR nummerierte Liste.",
			input,
		)
		if stream != nil {
			stream.AddStep("reason", "Aspekte:\n"+aspects, "reasoning")
		}
	}

	// Step 3: Analyze (depth >= 3)
	if depth >= 3 {
		if stream != nil {
			stream.AddStep("reason", "Tiefenanalyse...", "reasoning")
		}
		analysis = addon.thinkQuery(
			"Analysiere jeden der folgenden Aspekte einzeln. Betrachte Randfälle, Gegenargumente, Zusammenhänge.",
			fmt.Sprintf("Frage: %s\n\nAspekte:\n%s", input, aspects),
		)
		if stream != nil {
			stream.AddStep("reason", "Analyse:\n"+analysis, "reasoning")
		}
	}

	// Step 4: Evaluate (depth >= 4)
	if depth >= 4 {
		if stream != nil {
			stream.AddStep("reason", "Bewerte und prüfe...", "reasoning")
		}
		evaluation = addon.thinkQuery(
			"Prüfe die folgende Analyse kritisch: Gibt es Lücken? Widersprüche? Fehlende Perspektiven? Antworte kurz.",
			fmt.Sprintf("Frage: %s\n\nAnalyse:\n%s", input, analysis),
		)
		if stream != nil {
			stream.AddStep("reason", "Bewertung:\n"+evaluation, "reasoning")
		}
	}

	// Step 5: Challenge (depth >= 5)
	if depth >= 5 {
		if stream != nil {
			stream.AddStep("reason", "Gegenposition einnehmen...", "reasoning")
		}
		challenge = addon.thinkQuery(
			"Nimm die Gegenposition ein. Argumentiere GEGEN die bisherige Analyse. Was spricht dagegen? Welche alternativen Sichtweisen gibt es? Sei scharf und direkt.",
			fmt.Sprintf("Frage: %s\n\nBisherige Analyse:\n%s\n\nBewertung:\n%s", input, analysis, evaluation),
		)
		if stream != nil {
			stream.AddStep("reason", "Gegenposition:\n"+challenge, "reasoning")
		}
	}

	// Synthesize
	if stream != nil {
		stream.AddStep("reason", "Formuliere finale Antwort...", "reasoning")
	}

	var prior strings.Builder
	prior.WriteString("Verständnis:\n" + understanding)
	if aspects != "" {
		prior.WriteString("\n\nAspekte:\n" + aspects)
	}
	if analysis != "" {
		prior.WriteString("\n\nAnalyse:\n" + analysis)
	}
	if evaluation != "" {
		prior.WriteString("\n\nKritische Bewertung:\n" + evaluation)
	}
	if challenge != "" {
		prior.WriteString("\n\nGegenposition:\n" + challenge)
	}

	// Inject analysis as turn prompt — don't overwrite ctx.Input to avoid message duplication
	ctx.Prompts.Set(core.LayerTurn, "reasoning_analysis",
		fmt.Sprintf("Gründliche Voranalyse:\n%s\n\nNutze diese Analyse als Basis. Berücksichtige auch die Gegenargumente.", prior.String()), 85)
}

// runNative tells the provider to use the model's native reasoning (o3, Claude thinking).
// The provider reads the flag and adds reasoning_effort or extended_thinking to the API call.
// If the model doesn't support it, the flag is silently ignored.
func (addon *ReasoningAddon) runNative(ctx *core.Context) {
	stream := core.GetThinking(ctx)
	if stream != nil {
		stream.AddStep("reason", "Native Reasoning — delegiere an das Modell", "reasoning")
	}
	ctx.Set(KeyNativeReasoning, true)
}

// runReAct delegates to the Loop's tool cycle which already implements the reasoning.
func (addon *ReasoningAddon) runReAct(ctx *core.Context) {
	stream := core.GetThinking(ctx)
	if stream != nil {
		stream.AddStep("reason", "ReAct: Tool-basiertes Reasoning über den Loop", "reasoning")
	}
}

func (addon *ReasoningAddon) getDepth(ctx *core.Context, defaultDepth int) int {
	if val, ok := ctx.Get(KeyThinkingDepth); ok && val != nil {
		if depth, ok := val.(int); ok {
			return depth
		}
	}
	return defaultDepth
}

func (addon *ReasoningAddon) executeThinking(ctx *core.Context) {
	if addon.loop == nil {
		return
	}

	val, ok := ctx.Get(KeyStrategy)
	if !ok {
		return
	}
	strategy, ok := val.(Strategy)
	if !ok {
		return
	}

	switch strategy {
	case StrategyChainOfThought:
		depth := addon.getDepth(ctx, 3)
		if depth > 0 {
			addon.runChainOfThought(ctx, depth)
		}
	case StrategyDeepReasoning:
		depth := addon.getDepth(ctx, 5)
		if depth > 0 {
			addon.runDeepReasoning(ctx, depth)
		}
	case StrategyNative:
		addon.runNative(ctx)
	case StrategyReAct:
		addon.runReAct(ctx)
	}
}

func parseAspects(text string) []string {
	var aspects []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' {
			dotIdx := strings.Index(line, ".")
			if dotIdx >= 0 && dotIdx < 4 {
				aspect := strings.TrimSpace(line[dotIdx+1:])
				if aspect != "" {
					aspects = append(aspects, aspect)
				}
			}
		}
	}
	return aspects
}
