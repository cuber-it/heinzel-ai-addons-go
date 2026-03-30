package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewFeedbackAddon(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	if addon == nil {
		test.Fatal("NewFeedbackAddon returned nil")
	}
	if addon.Name() != "feedback" {
		test.Errorf("expected name 'feedback', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
	if len(addon.ratings) != 0 {
		test.Errorf("expected empty ratings, got %d", len(addon.ratings))
	}
	if len(addon.confidence) != 0 {
		test.Errorf("expected empty confidence, got %d", len(addon.confidence))
	}
}

func TestEmojiParsing(test *testing.T) {
	cases := []struct {
		input string
		score int
	}{
		{"👍", 1},
		{"👎", -1},
		{"⭐", 2},
		{"🗑️", -2},
		{"🗑", -2},
		{"👍👍", 2},
		{"👎👎", -2},
		{"+1", 1},
		{"-1", -1},
		{"++", 2},
		{"--", -2},
	}

	for _, tc := range cases {
		score, ok := emojiMap[tc.input]
		if !ok {
			test.Errorf("emoji %q not found in emojiMap", tc.input)
			continue
		}
		if score != tc.score {
			test.Errorf("emoji %q: expected score %d, got %d", tc.input, tc.score, score)
		}
	}
}

func TestRecordUpdatesConfidence(test *testing.T) {
	addon := NewFeedbackAddon(nil)

	rating := Rating{
		Target:   "response",
		TargetID: "msg:0",
		Score:    2,
	}
	addon.record(rating)

	score := addon.GetConfidence("response", "msg:0")
	// First rating: 0*0.7 + (2/2.0)*0.3 = 0.3
	if score < 0.29 || score > 0.31 {
		test.Errorf("expected confidence ~0.30 after +2 rating, got %.4f", score)
	}

	// Record a negative rating
	rating.Score = -2
	addon.record(rating)
	score = addon.GetConfidence("response", "msg:0")
	// Second: 0.3*0.7 + (-1.0)*0.3 = 0.21 - 0.30 = -0.09
	if score > 0.0 || score < -0.15 {
		test.Errorf("expected confidence ~-0.09 after negative rating, got %.4f", score)
	}
}

func TestRecordCallsOnStore(test *testing.T) {
	var stored []Rating
	addon := NewFeedbackAddon(func(rating Rating) {
		stored = append(stored, rating)
	})

	addon.record(Rating{Target: "test", TargetID: "1", Score: 1})
	addon.record(Rating{Target: "test", TargetID: "2", Score: -1})

	if len(stored) != 2 {
		test.Errorf("expected 2 stored ratings, got %d", len(stored))
	}
}

func TestHandleCommandRate(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()

	result := addon.HandleCommand("rate", "👍 workflow:deploy", ctx)
	if !strings.Contains(result, "workflow:deploy") {
		test.Errorf("expected target in result, got %q", result)
	}
	if !strings.Contains(result, "+1") {
		test.Errorf("expected score in result, got %q", result)
	}

	score := addon.GetConfidence("workflow", "deploy")
	if score == 0 {
		test.Error("expected non-zero confidence after rating")
	}
}

func TestHandleCommandRateNoArgs(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()

	result := addon.HandleCommand("rate", "", ctx)
	if !strings.Contains(result, "Usage") {
		test.Errorf("expected usage message, got %q", result)
	}
}

func TestHandleCommandRateUnknownEmoji(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()

	result := addon.HandleCommand("rate", "xyz", ctx)
	if !strings.Contains(result, "Unbekanntes Rating") {
		test.Errorf("expected unknown rating message, got %q", result)
	}
}

func TestHandleCommandConfidence(test *testing.T) {
	addon := NewFeedbackAddon(nil)

	// No ratings yet
	result := addon.HandleCommand("confidence", "", nil)
	if !strings.Contains(result, "No ratings") {
		test.Errorf("expected 'No ratings' message, got %q", result)
	}

	// Record some ratings
	addon.record(Rating{Target: "response", TargetID: "msg:0", Score: 2})
	addon.record(Rating{Target: "workflow", TargetID: "deploy", Score: -1})

	result = addon.HandleCommand("confidence", "", nil)
	if !strings.Contains(result, "Confidence scores") {
		test.Errorf("expected confidence scores header, got %q", result)
	}
	if !strings.Contains(result, "response:msg:0") {
		test.Errorf("expected response:msg:0 in output, got %q", result)
	}
}

func TestHandleCommandConfidenceSpecificTarget(test *testing.T) {
	addon := NewFeedbackAddon(nil)

	// Non-existent target
	result := addon.HandleCommand("confidence", "response:msg:99", nil)
	if !strings.Contains(result, "No ratings") {
		test.Errorf("expected 'No ratings' for unknown target, got %q", result)
	}

	// After recording
	addon.record(Rating{Target: "response", TargetID: "msg:0", Score: 2})
	result = addon.HandleCommand("confidence", "response:msg:0", nil)
	if !strings.Contains(result, "response:msg:0") {
		test.Errorf("expected target in output, got %q", result)
	}
}

func TestConfidenceInjectionLowScore(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()

	// Record enough negative ratings to get below -0.3
	for range 5 {
		addon.record(Rating{Target: "response", TargetID: "bad-pattern", Score: -2})
	}

	score := addon.GetConfidence("response", "bad-pattern")
	if score >= -0.3 {
		test.Fatalf("expected score below -0.3 for injection test, got %.4f", score)
	}

	addon.injectConfidence(ctx)

	// Check that a warning was injected into prompts
	composed := ctx.Prompts.Compose()
	if !strings.Contains(composed, "Quality Feedback") {
		test.Error("expected quality feedback warning in prompts")
	}
	if !strings.Contains(composed, "bad-pattern") {
		test.Error("expected bad-pattern mentioned in warning")
	}
}

func TestConfidenceInjectionSkipsInternal(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Set(core.KeyInternalQuery, true)

	// Record negative ratings
	for range 5 {
		addon.record(Rating{Target: "response", TargetID: "bad", Score: -2})
	}

	addon.injectConfidence(ctx)

	composed := ctx.Prompts.Compose()
	if strings.Contains(composed, "Quality Feedback") {
		test.Error("should not inject warnings for internal queries")
	}
}

func TestOnInputEmojiHaltsAndSetsOutput(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Input = "👍"
	ctx.Messages = []core.Message{{Role: "assistant", Content: "test response"}}

	addon.parseEmojiInput(ctx)

	if !ctx.Halt {
		test.Error("expected Halt after emoji input")
	}
	if ctx.Output == "" {
		test.Error("expected non-empty output after emoji input")
	}
}

func TestOnInputNegativeEmoji(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Input = "👎"
	ctx.Messages = []core.Message{{Role: "assistant", Content: "test"}}

	addon.parseEmojiInput(ctx)

	if !ctx.Halt {
		test.Error("expected Halt after negative emoji")
	}
	if !strings.Contains(ctx.Output, "besser") {
		test.Errorf("expected improvement message for negative emoji, got %q", ctx.Output)
	}
}

func TestOnInputStarEmoji(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Input = "⭐"
	ctx.Messages = []core.Message{{Role: "assistant", Content: "great response"}}

	addon.parseEmojiInput(ctx)

	if !ctx.Halt {
		test.Error("expected Halt after star emoji")
	}
	if !strings.Contains(ctx.Output, "besonders gut") {
		test.Errorf("expected 'besonders gut' for star emoji, got %q", ctx.Output)
	}
}

func TestOnInputRedoEmoji(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Input = "🔄"

	addon.parseEmojiInput(ctx)

	if !ctx.Halt {
		test.Error("expected Halt after redo emoji")
	}
	if !strings.Contains(ctx.Output, "wiederholt") {
		test.Errorf("expected redo message, got %q", ctx.Output)
	}
}

func TestOnInputNonEmojiDoesNotHalt(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	ctx := testContext()
	ctx.Input = "regular text input"

	addon.parseEmojiInput(ctx)

	if ctx.Halt {
		test.Error("regular text should not halt")
	}
	if ctx.Output != "" {
		test.Errorf("regular text should not set output, got %q", ctx.Output)
	}
}

func TestAllConfidence(test *testing.T) {
	addon := NewFeedbackAddon(nil)
	addon.record(Rating{Target: "a", TargetID: "1", Score: 1})
	addon.record(Rating{Target: "b", TargetID: "2", Score: -1})

	all := addon.AllConfidence()
	if len(all) != 2 {
		test.Errorf("expected 2 confidence entries, got %d", len(all))
	}
}
