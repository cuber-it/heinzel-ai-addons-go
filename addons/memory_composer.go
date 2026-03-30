// MemoryComposerAddon — orchestrates memory layers into context.

package addons

import (
	"fmt"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// MemoryComposerAddon wires the MemoryComposer into the hook system
type MemoryComposerAddon struct {
	core.BaseAddon
	composer *MemoryComposer
}

func NewMemoryComposerAddon(tokenBudget int) *MemoryComposerAddon {
	return &MemoryComposerAddon{
		composer: NewMemoryComposer(tokenBudget),
	}
}

func (addon *MemoryComposerAddon) Name() string           { return "memory-composer" }
func (addon *MemoryComposerAddon) Type() core.AddonType { return core.AddonMemory }
func (addon *MemoryComposerAddon) Start() error            { return nil }
func (addon *MemoryComposerAddon) Stop() error             { return nil }

func (addon *MemoryComposerAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnContextBuild, // compose and inject
	}
}

func (addon *MemoryComposerAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "memory", Description: "memory layer management", Usage: "/memory [stats|layers|compose]"},
	}
}

func (addon *MemoryComposerAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	switch args {
	case "", "stats":
		return addon.composer.Stats()
	case "layers":
		return addon.composer.Stats()
	case "compose":
		contribs := addon.composer.Compose(ctx)
		var lines []string
		totalTokens := 0
		for _, contrib := range contribs {
			preview := contrib.Content
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			lines = append(lines, fmt.Sprintf("  [%s] pri:%d ~%d tok → %s: %s",
				contrib.LayerName, contrib.Priority, contrib.TokenEstimate,
				contrib.Injection, preview))
			totalTokens += contrib.TokenEstimate
		}
		if len(lines) == 0 {
			return "No contributions."
		}
		return fmt.Sprintf("Contributions (%d, ~%d tokens):\n%s",
			len(lines), totalTokens, strings.Join(lines, "\n"))
	default:
		return "Usage: /memory [stats|layers|compose]"
	}
}

func (addon *MemoryComposerAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook == core.OnContextBuild {
		thinking := core.GetThinking(ctx)

		contribs := addon.composer.Compose(ctx)
		if len(contribs) > 0 {
			addon.composer.Apply(ctx, contribs)
			if thinking != nil {
				totalTokens := 0
				for _, contrib := range contribs {
					totalTokens += contrib.TokenEstimate
				}
				thinking.AddStep("memory", fmt.Sprintf("%d layers, ~%d tokens injected",
					len(contribs), totalTokens), "memory-composer")
			}
		}
	}
	return core.Result{}
}

func (addon *MemoryComposerAddon) Composer() *MemoryComposer {
	return addon.composer
}

// === Built-in Memory Layers ===

// FactsLayer injects persistent facts into system prompt
type FactsLayer struct {
	facts map[string]string // key → value
}

func NewFactsLayer() *FactsLayer {
	return &FactsLayer{facts: make(map[string]string)}
}

func (layer *FactsLayer) LayerName() string  { return "facts" }
func (layer *FactsLayer) LayerPriority() int { return 90 }
func (layer *FactsLayer) CanCompact() bool   { return false }
func (layer *FactsLayer) Compact(ctx *core.Context) error { return nil }

func (layer *FactsLayer) Retrieve(ctx *core.Context) MemoryContribution {
	if len(layer.facts) == 0 {
		return MemoryContribution{}
	}
	var lines []string
	lines = append(lines, "Known facts:")
	for key, val := range layer.facts {
		lines = append(lines, fmt.Sprintf("- %s: %s", key, val))
	}
	content := strings.Join(lines, "\n")
	return MemoryContribution{
		LayerName:     "facts",
		Content:       content,
		TokenEstimate: len(content) / 4,
		Priority:      90,
		Injection:     InjectSystemPrompt,
		Compactable:   false,
	}
}

func (layer *FactsLayer) Set(key, value string)  { layer.facts[key] = value }
func (layer *FactsLayer) Get(key string) string   { return layer.facts[key] }
func (layer *FactsLayer) Delete(key string)        { delete(layer.facts, key) }
func (layer *FactsLayer) Count() int              { return len(layer.facts) }

// SessionSummaryLayer holds compressed summaries of past conversation
type SessionSummaryLayer struct {
	summaries []string
	maxTokens int
}

func NewSessionSummaryLayer(maxTokens int) *SessionSummaryLayer {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	return &SessionSummaryLayer{maxTokens: maxTokens}
}

func (layer *SessionSummaryLayer) LayerName() string  { return "session_summary" }
func (layer *SessionSummaryLayer) LayerPriority() int { return 60 }
func (layer *SessionSummaryLayer) CanCompact() bool   { return true }

func (layer *SessionSummaryLayer) Compact(ctx *core.Context) error {
	// Drop oldest summaries
	if len(layer.summaries) > 1 {
		layer.summaries = layer.summaries[1:]
	}
	return nil
}

func (layer *SessionSummaryLayer) Retrieve(ctx *core.Context) MemoryContribution {
	if len(layer.summaries) == 0 {
		return MemoryContribution{}
	}
	content := "Previous context:\n" + strings.Join(layer.summaries, "\n---\n")
	// Trim to budget
	if len(content)/4 > layer.maxTokens {
		content = content[:layer.maxTokens*4]
	}
	return MemoryContribution{
		LayerName:     "session_summary",
		Content:       content,
		TokenEstimate: len(content) / 4,
		Priority:      60,
		Injection:     InjectSystemPrompt,
		Compactable:   true,
	}
}

func (layer *SessionSummaryLayer) AddSummary(summary string) {
	layer.summaries = append(layer.summaries, summary)
}

// RecentMessagesLayer injects recent conversation messages
// This is the "working memory" — raw messages, not summarized
type RecentMessagesLayer struct {
	maxMessages int
}

func NewRecentMessagesLayer(maxMessages int) *RecentMessagesLayer {
	if maxMessages <= 0 {
		maxMessages = 50
	}
	return &RecentMessagesLayer{maxMessages: maxMessages}
}

func (layer *RecentMessagesLayer) LayerName() string  { return "recent" }
func (layer *RecentMessagesLayer) LayerPriority() int { return 70 }
func (layer *RecentMessagesLayer) CanCompact() bool   { return true }

func (layer *RecentMessagesLayer) Compact(ctx *core.Context) error {
	// Halve the window
	layer.maxMessages = layer.maxMessages / 2
	if layer.maxMessages < 4 {
		layer.maxMessages = 4
	}
	return nil
}

func (layer *RecentMessagesLayer) Retrieve(ctx *core.Context) MemoryContribution {
	msgs := ctx.Messages
	if len(msgs) == 0 {
		return MemoryContribution{}
	}

	start := 0
	if len(msgs) > layer.maxMessages {
		start = len(msgs) - layer.maxMessages
	}
	recent := msgs[start:]

	tokenEst := 0
	for _, msg := range recent {
		tokenEst += len(msg.Content) / 4
	}

	return MemoryContribution{
		LayerName:     "recent",
		TokenEstimate: tokenEst,
		Priority:      70,
		Injection:     InjectMessages,
		Compactable:   true,
		Messages:      recent,
	}
}

// ToolsLayer injects available tool descriptions
type ToolsLayer struct{}

func (layer *ToolsLayer) LayerName() string  { return "tools" }
func (layer *ToolsLayer) LayerPriority() int { return 40 }
func (layer *ToolsLayer) CanCompact() bool   { return false }
func (layer *ToolsLayer) Compact(ctx *core.Context) error { return nil }

func (layer *ToolsLayer) Retrieve(ctx *core.Context) MemoryContribution {
	val, ok := ctx.Get(KeyToolRegistry)
	if !ok {
		return MemoryContribution{}
	}
	reg, ok := val.(*core.ToolRegistry)
	if !ok || reg.Count() == 0 {
		return MemoryContribution{}
	}

	var lines []string
	lines = append(lines, "Available tools:")
	for _, tool := range reg.All() {
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}
	content := strings.Join(lines, "\n")
	return MemoryContribution{
		LayerName:     "tools",
		Content:       content,
		TokenEstimate: len(content) / 4,
		Priority:      40,
		Injection:     InjectSystemPrompt,
		Compactable:   false,
	}
}
