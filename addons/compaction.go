// CompactionAddon — context compaction via fact extraction and summarization.

package addons

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const (
	defaultCompactAfterMessages = 30
	defaultSummaryChunkSize     = 20
)

// MemorySink is where compacted knowledge goes for persistence
type MemorySink interface {
	StoreFact(key, value, source string) error
	StoreSummary(sessionID, summary string, timestamp time.Time) error
}

// CompactionAddon implements intelligent context compaction
// ALL LLM calls go through the loop — no direct API access
type CompactionAddon struct {
	core.BaseAddon
	loop                 *core.Loop
	factsLayer           *FactsLayer
	summaryLayer         *SessionSummaryLayer
	sinks                []MemorySink
	compactAfterMessages int
	summaryChunkSize     int
	extractPrompt        string
	summarizePrompt      string
}

func NewCompactionAddon(factsLayer *FactsLayer, summaryLayer *SessionSummaryLayer) *CompactionAddon {
	return &CompactionAddon{
		factsLayer:           factsLayer,
		summaryLayer:         summaryLayer,
		sinks:                make([]MemorySink, 0),
		compactAfterMessages: defaultCompactAfterMessages,
		summaryChunkSize:     defaultSummaryChunkSize,
		extractPrompt: `Analysiere den folgenden Gesprächsausschnitt.
Extrahiere ALLE wichtigen Fakten als Key-Value-Paare.
Fakten sind: Namen, Zahlen, Entscheidungen, Präferenzen, technische Details, Konfigurationen.
Antworte NUR als JSON-Array: [{"key": "...", "value": "..."}]
Keine Erklärung, nur JSON.`,
		summarizePrompt: `Fasse den folgenden Gesprächsausschnitt zusammen.
Behalte:
- Alle Entscheidungen und deren Begründung
- Offene Fragen und TODOs
- Wer was gesagt/gewollt hat
- Technische Details die noch relevant sein könnten
Verliere: Grüße, Smalltalk, Wiederholungen, Zwischenschritte die zum Ergebnis geführt haben.
Maximal 500 Wörter.`,
	}
}

// SetLoop connects the compaction addon to the main loop
// Must be called after loop creation, before use
func (addon *CompactionAddon) SetLoop(loop *core.Loop) {
	addon.loop = loop
}

func (addon *CompactionAddon) AddSink(sink MemorySink) {
	addon.sinks = append(addon.sinks, sink)
}

func (addon *CompactionAddon) Name() string           { return "compaction" }
func (addon *CompactionAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *CompactionAddon) Start() error            { return nil }
func (addon *CompactionAddon) Stop() error             { return nil }

func (addon *CompactionAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnContextOverflow,
		core.OnLoopEnd,
	}
}

func (addon *CompactionAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "compact", Description: "trigger compaction",
			Usage: "compact [extract|summarize|status]"},
	}
}

func (addon *CompactionAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	switch args {
	case "", "status":
		return fmt.Sprintf("Messages: %d (compact after %d)\nFacts: %d\nEstimated tokens: %d / %d\nOver budget: %v",
			len(ctx.Messages), addon.compactAfterMessages,
			addon.factsLayer.Count(),
			ctx.TokenEstimate(), ctx.TokenBudget,
			ctx.OverBudget())
	case "extract":
		n := addon.extractFacts(ctx)
		return fmt.Sprintf("Extracted %d facts.", n)
	case "summarize":
		addon.summarizeOldMessages(ctx)
		return "Summarized old messages."
	}
	return "Usage: compact [extract|summarize|status]"
}

func (addon *CompactionAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnContextOverflow:
		thinking := core.GetThinking(ctx)
		if thinking != nil {
			thinking.AddStep("memory", "context overflow — compacting", "compaction")
		}
		addon.fullCompaction(ctx)

	case core.OnLoopEnd:
		msgCount := len(ctx.Messages)
		if msgCount > addon.compactAfterMessages {
			lastCompacted := 0
			if val, ok := ctx.Get(KeyCompactionLastMsgCount); ok {
				if lc, ok := val.(int); ok {
					lastCompacted = lc
				}
			}
			if msgCount-lastCompacted >= addon.compactAfterMessages {
				thinking := core.GetThinking(ctx)
				if thinking != nil {
					thinking.AddStep("memory", fmt.Sprintf("proactive compaction at %d messages", msgCount), "compaction")
				}
				addon.extractFacts(ctx)
				addon.summarizeOldMessages(ctx)
				ctx.Set(KeyCompactionLastMsgCount, msgCount)
			}
		}
	}
	return core.Result{}
}

func (addon *CompactionAddon) fullCompaction(ctx *core.Context) {
	addon.extractFacts(ctx)
	addon.summarizeOldMessages(ctx)
	if ctx.OverBudget() && addon.summaryLayer != nil {
		addon.summaryLayer.Compact(ctx)
	}
	if ctx.OverBudget() {
		addon.rollingHandover(ctx)
	}
}

// === Internal LLM calls — go through the loop, not direct API ===

// internalQuery runs a hidden turn through the loop for compaction purposes
func (addon *CompactionAddon) internalQuery(systemPrompt, userMessage string) string {
	if addon.loop == nil {
		return ""
	}

	// Create isolated context — doesn't pollute the main conversation
	ctx := core.NewContext("compaction")
	ctx.SystemPrompt = systemPrompt
	core.InitThinking(ctx, nil) // silent

	// Mark as internal so addons don't log this as a user turn
	ctx.Set(KeyInternalQuery, true)

	output := addon.loop.Run(ctx, userMessage)
	return output
}

func (addon *CompactionAddon) extractFacts(ctx *core.Context) int {
	if len(ctx.Messages) < 4 {
		return 0
	}

	lastExtracted := 0
	if val, ok := ctx.Get(KeyCompactionLastExtracted); ok {
		if idx, ok := val.(int); ok {
			lastExtracted = idx
		}
	}
	if lastExtracted >= len(ctx.Messages) {
		return 0
	}

	var convLines []string
	for idx := lastExtracted; idx < len(ctx.Messages); idx++ {
		msg := ctx.Messages[idx]
		convLines = append(convLines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}
	conversation := strings.Join(convLines, "\n")
	if len(conversation) < 100 {
		return 0
	}

	// LLM call through the loop
	response := addon.internalQuery(addon.extractPrompt, conversation)
	if response == "" {
		return 0
	}

	// Parse JSON
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		if len(lines) > 2 {
			response = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	type extractedFact struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	var facts []extractedFact
	if err := json.Unmarshal([]byte(response), &facts); err != nil {
		return 0 // LLM response was not valid JSON
	}

	for _, fact := range facts {
		addon.factsLayer.Set(fact.Key, fact.Value)
		for _, sink := range addon.sinks {
			sink.StoreFact(fact.Key, fact.Value, ctx.SessionID)
		}
	}

	ctx.Set(KeyCompactionLastExtracted, len(ctx.Messages))
	return len(facts)
}

func (addon *CompactionAddon) summarizeOldMessages(ctx *core.Context) {
	if len(ctx.Messages) <= addon.summaryChunkSize {
		return
	}

	chunk := ctx.Messages[:addon.summaryChunkSize]
	var convLines []string
	for _, msg := range chunk {
		convLines = append(convLines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}

	// LLM call through the loop
	summary := addon.internalQuery(addon.summarizePrompt, strings.Join(convLines, "\n"))
	if summary != "" {
		if addon.summaryLayer != nil {
			addon.summaryLayer.AddSummary(summary)
		}
		for _, sink := range addon.sinks {
			sink.StoreSummary(ctx.SessionID, summary, time.Now())
		}
	}

	ctx.Messages = ctx.Messages[addon.summaryChunkSize:]
}

func (addon *CompactionAddon) rollingHandover(ctx *core.Context) {
	thinking := core.GetThinking(ctx)
	if thinking != nil {
		thinking.AddStep("memory", "rolling handover — context reset with continuity", "compaction")
	}

	ctx.Set(KeyCompactionLastExtracted, 0)
	addon.extractFacts(ctx)

	var allLines []string
	for _, msg := range ctx.Messages {
		allLines = append(allLines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}

	handoverSummary := addon.internalQuery(addon.summarizePrompt, strings.Join(allLines, "\n"))

	for _, sink := range addon.sinks {
		sink.StoreSummary(ctx.SessionID, "HANDOVER: "+handoverSummary, time.Now())
	}

	var handoverParts []string
	handoverParts = append(handoverParts, "=== Session Continuity ===")
	if addon.summaryLayer != nil {
		for _, summary := range addon.summaryLayer.summaries {
			handoverParts = append(handoverParts, summary)
		}
	}
	if handoverSummary != "" {
		handoverParts = append(handoverParts, "--- Aktuellster Kontext ---")
		handoverParts = append(handoverParts, handoverSummary)
	}

	ctx.Messages = nil
	ctx.Output = ""
	ctx.MemoryResults = make(map[string]interface{})
	ctx.Set(KeyCompactionLastExtracted, 0)
	ctx.Prompts.Set(core.LayerSession, "handover", strings.Join(handoverParts, "\n"), 80)

	if addon.summaryLayer != nil {
		addon.summaryLayer.summaries = nil
	}

	if thinking != nil {
		thinking.AddStep("memory", fmt.Sprintf("handover complete — %d facts preserved, context reset",
			addon.factsLayer.Count()), "compaction")
	}
}
