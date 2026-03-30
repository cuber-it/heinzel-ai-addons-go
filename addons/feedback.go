// FeedbackAddon — quality feedback and rating system.
// Users rate responses, workflows, rules via emoji (👍👎⭐🗑️🔄).
// Ratings accumulate into confidence scores that influence future behavior.

package addons

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// Rating represents a user quality judgment
type Rating struct {
	Target    string    // what was rated (response, workflow, rule, fact)
	TargetID  string    // identifier (message index, workflow name, fact key)
	Score     int       // -2 to +2 (👎👎=-2, 👎=-1, 👍=+1, 👍👍=+2, ⭐=+2)
	Timestamp time.Time
	Context   string    // what was the conversation context
}

// FeedbackAddon tracks quality ratings and builds confidence scores
type FeedbackAddon struct {
	core.BaseAddon
	mu         sync.RWMutex
	ratings    []Rating
	confidence map[string]float64 // targetID → accumulated confidence (-1.0 to +1.0)
	onStore    func(rating Rating) // callback to persist (e.g. to SQLite or Prolog)
}

func NewFeedbackAddon(onStore func(Rating)) *FeedbackAddon {
	return &FeedbackAddon{
		ratings:    make([]Rating, 0),
		confidence: make(map[string]float64),
		onStore:    onStore,
	}
}

func (addon *FeedbackAddon) Name() string           { return "feedback" }
func (addon *FeedbackAddon) Type() core.AddonType   { return core.AddonFilter }
func (addon *FeedbackAddon) Start() error            { return nil }
func (addon *FeedbackAddon) Stop() error             { return nil }

func (addon *FeedbackAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnInput, core.OnContextBuild}
}

func (addon *FeedbackAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "rate", Description: "rate last response or a target",
			Usage: "rate [👍|👎|⭐|🗑️] [target]"},
		{Name: "confidence", Description: "show confidence scores",
			Usage: "confidence [target]"},
	}
}

func (addon *FeedbackAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	switch cmd {
	case "rate":
		return addon.handleRate(args, ctx)
	case "confidence":
		return addon.handleConfidence(args)
	}
	return ""
}

func (addon *FeedbackAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnInput:
		addon.parseEmojiInput(ctx)
	case core.OnContextBuild:
		addon.injectConfidence(ctx)
	}
	return core.Result{}
}

// === Emoji parsing ===

var emojiMap = map[string]int{
	"👍":  1,
	"👍👍": 2,
	"👎":  -1,
	"👎👎": -2,
	"⭐":  2,
	"🗑️": -2,
	"🗑":  -2,
	"🔄":  0, // special: redo
	"+1":  1,
	"-1":  -1,
	"++":  2,
	"--":  -2,
}

func (addon *FeedbackAddon) parseEmojiInput(ctx *core.Context) {
	input := strings.TrimSpace(ctx.Input)

	if input == "🔄" || input == "/redo" {
		ctx.Set("needs_redo", true)
		ctx.Halt = true
		ctx.Output = "Wird wiederholt..."
		return
	}

	score, isRating := emojiMap[input]
	if !isRating {
		return
	}

	lastIdx := len(ctx.Messages) - 1
	if lastIdx < 0 {
		return
	}

	rating := Rating{
		Target:    "response",
		TargetID:  fmt.Sprintf("msg:%d", lastIdx),
		Score:     score,
		Timestamp: time.Now(),
		Context:   ctx.SessionID,
	}

	addon.record(rating)

	ctx.Halt = true
	feedback := "👍"
	if score < 0 {
		feedback = "Verstanden, wird besser."
	} else if score >= 2 {
		feedback = "Gespeichert als besonders gut!"
	}
	ctx.Output = feedback
}

// === Rating ===

func (addon *FeedbackAddon) handleRate(args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "Usage: rate [👍|👎|⭐|🗑️] [workflow:name | fact:key | last]"
	}

	score, ok := emojiMap[parts[0]]
	if !ok {
		return fmt.Sprintf("Unbekanntes Rating: %s", parts[0])
	}

	target := "response"
	targetID := "last"
	if len(parts) > 1 {
		targetStr := parts[1]
		if strings.Contains(targetStr, ":") {
			split := strings.SplitN(targetStr, ":", 2)
			target = split[0]
			targetID = split[1]
		} else {
			targetID = targetStr
		}
	}

	rating := Rating{
		Target:    target,
		TargetID:  targetID,
		Score:     score,
		Timestamp: time.Now(),
		Context:   ctx.SessionID,
	}

	addon.record(rating)
	return fmt.Sprintf("Rated %s:%s → %+d", target, targetID, score)
}

func (addon *FeedbackAddon) record(rating Rating) {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	addon.ratings = append(addon.ratings, rating)

	// Exponential moving average: 70% old + 30% new (normalized to -1..+1)
	key := rating.Target + ":" + rating.TargetID
	current := addon.confidence[key]
	normalized := float64(rating.Score) / 2.0
	addon.confidence[key] = current*0.7 + normalized*0.3

	if addon.onStore != nil {
		addon.onStore(rating)
	}
}

// === Confidence ===

func (addon *FeedbackAddon) handleConfidence(args string) string {
	addon.mu.RLock()
	defer addon.mu.RUnlock()

	if args != "" {
		key := args
		if !strings.Contains(key, ":") {
			key = "response:" + key
		}
		score, ok := addon.confidence[key]
		if !ok {
			return fmt.Sprintf("No ratings for %s", key)
		}
		return fmt.Sprintf("%s: %.2f (%s)", key, score, confidenceLabel(score))
	}

	if len(addon.confidence) == 0 {
		return "No ratings recorded."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Confidence scores (%d):", len(addon.confidence)))
	for key, score := range addon.confidence {
		lines = append(lines, fmt.Sprintf("  %-30s %+.2f (%s)", key, score, confidenceLabel(score)))
	}
	return strings.Join(lines, "\n")
}

func confidenceLabel(score float64) string {
	switch {
	case score > 0.5:
		return "high"
	case score > 0.1:
		return "good"
	case score > -0.1:
		return "neutral"
	case score > -0.5:
		return "low"
	default:
		return "poor"
	}
}

// === Confidence injection into context ===

func (addon *FeedbackAddon) injectConfidence(ctx *core.Context) {
	if val, ok := ctx.Get(KeyInternalQuery); ok {
		if internal, ok := val.(bool); ok && internal {
			return
		}
	}

	addon.mu.RLock()
	defer addon.mu.RUnlock()

	var warnings []string
	for key, score := range addon.confidence {
		if score < -0.3 {
			warnings = append(warnings, fmt.Sprintf("  - %s (confidence: %.2f) — der User war unzufrieden, vermeide dieses Muster", key, score))
		}
	}

	if len(warnings) > 0 {
		ctx.Prompts.Set(core.LayerTurn, "feedback_warnings",
			"Quality Feedback:\n"+strings.Join(warnings, "\n"), 35)
	}
}

func (addon *FeedbackAddon) GetConfidence(target, targetID string) float64 {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	return addon.confidence[target+":"+targetID]
}

func (addon *FeedbackAddon) AllConfidence() map[string]float64 {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	result := make(map[string]float64, len(addon.confidence))
	for k, v := range addon.confidence {
		result[k] = v
	}
	return result
}
