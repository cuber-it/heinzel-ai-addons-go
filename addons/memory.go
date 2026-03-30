// MemoryLayer and MemoryComposer — memory contribution system.
package addons

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// InjectionPoint defines where a memory contribution goes
type InjectionPoint int

const (
	InjectSystemPrompt InjectionPoint = iota // goes into system prompt
	InjectMessages                           // goes as messages into conversation
	InjectMetadata                           // goes into ctx.State for other addons
)

var injectionNames = [...]string{"system_prompt", "messages", "metadata"}

func (ip InjectionPoint) String() string {
	if int(ip) < len(injectionNames) {
		return injectionNames[ip]
	}
	return "unknown"
}

// MemoryContribution is what a memory layer returns for one turn
type MemoryContribution struct {
	LayerName     string
	Content       string         // text content
	TokenEstimate int            // estimated tokens (len/4)
	Priority      int            // higher = more important
	Injection     InjectionPoint // where to inject
	Compactable   bool           // can be shortened if budget tight
	Messages      []core.Message  // for InjectMessages
}

func (mc *MemoryContribution) IsEmpty() bool {
	return mc.Content == "" && len(mc.Messages) == 0
}

// MemoryLayer is the interface for a memory source
type MemoryLayer interface {
	// Identity
	LayerName() string
	LayerPriority() int

	// Retrieve context for this turn
	Retrieve(ctx *core.Context) MemoryContribution

	// Compact internal state (summarize, trim)
	Compact(ctx *core.Context) error

	// Whether this layer supports compaction
	CanCompact() bool
}

// MemoryComposer orchestrates all memory layers
type MemoryComposer struct {
	layers      []MemoryLayer
	totalBudget int // token budget for all memory layers combined
}

func NewMemoryComposer(totalBudget int) *MemoryComposer {
	if totalBudget <= 0 {
		totalBudget = 16000
	}
	return &MemoryComposer{
		totalBudget: totalBudget,
	}
}

func (mc *MemoryComposer) Register(layer MemoryLayer) {
	mc.layers = append(mc.layers, layer)
	sort.Slice(mc.layers, func(i, j int) bool {
		return mc.layers[i].LayerPriority() > mc.layers[j].LayerPriority()
	})
}

func (mc *MemoryComposer) Unregister(name string) {
	var kept []MemoryLayer
	for _, layer := range mc.layers {
		if layer.LayerName() != name {
			kept = append(kept, layer)
		}
	}
	mc.layers = kept
}

func (mc *MemoryComposer) Layers() []MemoryLayer {
	return mc.layers
}

func (mc *MemoryComposer) Compose(ctx *core.Context) []MemoryContribution {
	var contributions []MemoryContribution
	for _, layer := range mc.layers {
		contrib := layer.Retrieve(ctx)
		if contrib.IsEmpty() {
			continue
		}
		if contrib.TokenEstimate == 0 {
			contrib.TokenEstimate = len(contrib.Content)/4 + len(contrib.Messages)*50
		}
		contributions = append(contributions, contrib)
	}

	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].Priority > contributions[j].Priority
	})

	usedTokens := 0
	var accepted []MemoryContribution
	var compactable []int // indices of compactable contributions that made it in

	for _, contrib := range contributions {
		if !contrib.Compactable {
			// Non-compactable always goes in
			accepted = append(accepted, contrib)
			usedTokens += contrib.TokenEstimate
			continue
		}

		if usedTokens+contrib.TokenEstimate <= mc.totalBudget {
			accepted = append(accepted, contrib)
			compactable = append(compactable, len(accepted)-1)
			usedTokens += contrib.TokenEstimate
		}
	}

	// If still over budget, compact lowest-priority compactable layers
	if usedTokens > mc.totalBudget && len(compactable) > 0 {
		for idx := len(compactable) - 1; idx >= 0; idx-- {
			layerIdx := compactable[idx]
			layerName := accepted[layerIdx].LayerName

			// Find the layer and compact it
			for _, layer := range mc.layers {
				if layer.LayerName() == layerName && layer.CanCompact() {
					layer.Compact(ctx)
					// Re-retrieve after compaction
					newContrib := layer.Retrieve(ctx)
					if !newContrib.IsEmpty() {
						if newContrib.TokenEstimate == 0 {
							newContrib.TokenEstimate = len(newContrib.Content) / 4
						}
						usedTokens -= accepted[layerIdx].TokenEstimate
						usedTokens += newContrib.TokenEstimate
						accepted[layerIdx] = newContrib
					}
					break
				}
			}

			if usedTokens <= mc.totalBudget {
				break
			}
		}
	}

	return accepted
}

func (mc *MemoryComposer) Apply(ctx *core.Context, contributions []MemoryContribution) {
	for _, contrib := range contributions {
		switch contrib.Injection {
		case InjectSystemPrompt:
			ctx.Prompts.Add(core.LayerTurn, "memory:"+contrib.LayerName, contrib.Content, contrib.Priority)

		case InjectMessages:
			for _, msg := range contrib.Messages {
				ctx.Messages = append(ctx.Messages, msg)
			}

		case InjectMetadata:
			ctx.Set("memory:"+contrib.LayerName, contrib.Content)
		}
	}
}

func (mc *MemoryComposer) Stats() string {
	var lines []string
	for _, layer := range mc.layers {
		compact := ""
		if layer.CanCompact() {
			compact = " (compactable)"
		}
		lines = append(lines, fmt.Sprintf("  %-20s priority:%d%s", layer.LayerName(), layer.LayerPriority(), compact))
	}
	if len(lines) == 0 {
		return "No memory layers registered."
	}
	return fmt.Sprintf("Memory layers (%d), budget: %d tokens:\n%s",
		len(mc.layers), mc.totalBudget, strings.Join(lines, "\n"))
}
