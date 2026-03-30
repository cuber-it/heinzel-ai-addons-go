package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestFactsLayerStoreAndRetrieve(test *testing.T) {
	layer := NewFactsLayer()

	layer.Set("user_name", "ucuber")
	layer.Set("language", "Go")

	if layer.Count() != 2 {
		test.Errorf("expected 2 facts, got %d", layer.Count())
	}
	if layer.Get("user_name") != "ucuber" {
		test.Errorf("expected 'ucuber', got %q", layer.Get("user_name"))
	}

	ctx := testContext()
	contrib := layer.Retrieve(ctx)

	if contrib.IsEmpty() {
		test.Error("expected non-empty contribution from facts layer")
	}
	if contrib.LayerName != "facts" {
		test.Errorf("expected layer name 'facts', got %q", contrib.LayerName)
	}
	if contrib.Priority != 90 {
		test.Errorf("expected priority 90, got %d", contrib.Priority)
	}
	if contrib.Injection != InjectSystemPrompt {
		test.Errorf("expected InjectSystemPrompt, got %v", contrib.Injection)
	}
	if !strings.Contains(contrib.Content, "ucuber") {
		test.Error("expected content to contain 'ucuber'")
	}
	if !strings.Contains(contrib.Content, "Go") {
		test.Error("expected content to contain 'Go'")
	}
	if contrib.Compactable {
		test.Error("facts layer should not be compactable")
	}
}

func TestFactsLayerEmptyRetrieve(test *testing.T) {
	layer := NewFactsLayer()
	ctx := testContext()
	contrib := layer.Retrieve(ctx)

	if !contrib.IsEmpty() {
		test.Error("expected empty contribution when no facts stored")
	}
}

func TestFactsLayerDelete(test *testing.T) {
	layer := NewFactsLayer()
	layer.Set("key1", "val1")
	layer.Set("key2", "val2")
	layer.Delete("key1")

	if layer.Count() != 1 {
		test.Errorf("expected 1 fact after delete, got %d", layer.Count())
	}
	if layer.Get("key1") != "" {
		test.Error("expected deleted key to return empty string")
	}
}

func TestRecentMessagesLayerReturnsLastN(test *testing.T) {
	layer := NewRecentMessagesLayer(3)
	ctx := testContext()

	// Add 5 messages
	for idx := 0; idx < 5; idx++ {
		ctx.Messages = append(ctx.Messages, core.Message{
			Role:    "user",
			Content: strings.Repeat("x", 100),
		})
	}

	contrib := layer.Retrieve(ctx)
	if contrib.IsEmpty() {
		test.Fatal("expected non-empty contribution")
	}
	if len(contrib.Messages) != 3 {
		test.Errorf("expected 3 messages (last N), got %d", len(contrib.Messages))
	}
	if contrib.LayerName != "recent" {
		test.Errorf("expected layer name 'recent', got %q", contrib.LayerName)
	}
	if contrib.Injection != InjectMessages {
		test.Errorf("expected InjectMessages, got %v", contrib.Injection)
	}
	if !contrib.Compactable {
		test.Error("recent messages layer should be compactable")
	}
}

func TestRecentMessagesLayerEmptyMessages(test *testing.T) {
	layer := NewRecentMessagesLayer(10)
	ctx := testContext()

	contrib := layer.Retrieve(ctx)
	if !contrib.IsEmpty() {
		test.Error("expected empty contribution when no messages")
	}
}

func TestRecentMessagesLayerCompactHalvesWindow(test *testing.T) {
	layer := NewRecentMessagesLayer(20)
	ctx := testContext()

	layer.Compact(ctx)
	if layer.maxMessages != 10 {
		test.Errorf("expected maxMessages halved to 10, got %d", layer.maxMessages)
	}

	layer.Compact(ctx)
	if layer.maxMessages != 5 {
		test.Errorf("expected maxMessages halved to 5, got %d", layer.maxMessages)
	}

	// Should not go below 4
	layer.Compact(ctx)
	layer.Compact(ctx)
	if layer.maxMessages < 4 {
		test.Errorf("expected maxMessages minimum 4, got %d", layer.maxMessages)
	}
}

func TestRecentMessagesLayerFewerThanMax(test *testing.T) {
	layer := NewRecentMessagesLayer(10)
	ctx := testContext()

	ctx.Messages = append(ctx.Messages, core.Message{Role: "user", Content: "hello"})
	ctx.Messages = append(ctx.Messages, core.Message{Role: "assistant", Content: "hi"})

	contrib := layer.Retrieve(ctx)
	if len(contrib.Messages) != 2 {
		test.Errorf("expected 2 messages (fewer than max), got %d", len(contrib.Messages))
	}
}

func TestSessionSummaryLayerStoresAndRetrieves(test *testing.T) {
	layer := NewSessionSummaryLayer(2000)

	layer.AddSummary("We discussed Go testing patterns.")
	layer.AddSummary("User prefers table-driven tests.")

	ctx := testContext()
	contrib := layer.Retrieve(ctx)

	if contrib.IsEmpty() {
		test.Fatal("expected non-empty contribution")
	}
	if contrib.LayerName != "session_summary" {
		test.Errorf("expected layer name 'session_summary', got %q", contrib.LayerName)
	}
	if contrib.Priority != 60 {
		test.Errorf("expected priority 60, got %d", contrib.Priority)
	}
	if !strings.Contains(contrib.Content, "Go testing") {
		test.Error("expected content to contain first summary")
	}
	if !strings.Contains(contrib.Content, "table-driven") {
		test.Error("expected content to contain second summary")
	}
	if !contrib.Compactable {
		test.Error("session summary should be compactable")
	}
}

func TestSessionSummaryLayerEmptyRetrieve(test *testing.T) {
	layer := NewSessionSummaryLayer(2000)
	ctx := testContext()
	contrib := layer.Retrieve(ctx)

	if !contrib.IsEmpty() {
		test.Error("expected empty contribution when no summaries")
	}
}

func TestSessionSummaryLayerCompactDropsOldest(test *testing.T) {
	layer := NewSessionSummaryLayer(2000)
	layer.AddSummary("First summary")
	layer.AddSummary("Second summary")
	layer.AddSummary("Third summary")

	ctx := testContext()
	layer.Compact(ctx)

	if len(layer.summaries) != 2 {
		test.Errorf("expected 2 summaries after compact, got %d", len(layer.summaries))
	}
	// First should be dropped
	if layer.summaries[0] != "Second summary" {
		test.Errorf("expected 'Second summary' as first after compact, got %q", layer.summaries[0])
	}
}

func TestMemoryComposerComposesWithinBudget(test *testing.T) {
	composer := NewMemoryComposer(500) // small budget

	factsLayer := NewFactsLayer()
	factsLayer.Set("name", "ucuber")

	summaryLayer := NewSessionSummaryLayer(500)
	summaryLayer.AddSummary(strings.Repeat("summary text ", 100)) // large summary

	composer.Register(factsLayer)
	composer.Register(summaryLayer)

	ctx := testContext()
	contribs := composer.Compose(ctx)

	// Facts are non-compactable so always included; summary might be trimmed or excluded
	factsFound := false
	for _, contrib := range contribs {
		if contrib.LayerName == "facts" {
			factsFound = true
		}
	}
	if !factsFound {
		test.Error("expected facts layer to always be included (non-compactable)")
	}
}

func TestMemoryComposerAddonComposerAccess(test *testing.T) {
	addon := NewMemoryComposerAddon(8000)

	if addon.Name() != "memory-composer" {
		test.Errorf("expected name 'memory-composer', got %q", addon.Name())
	}
	if addon.Type() != core.AddonMemory {
		test.Errorf("expected type AddonMemory, got %v", addon.Type())
	}
	if addon.Composer() == nil {
		test.Error("expected non-nil composer")
	}
}

func TestMemoryComposerAddonHandleOnContextBuild(test *testing.T) {
	addon := NewMemoryComposerAddon(8000)
	factsLayer := NewFactsLayer()
	factsLayer.Set("project", "neo-heinzel")
	addon.Composer().Register(factsLayer)

	ctx := testContext()
	addon.Handle(core.OnContextBuild, ctx)

	// Facts should be injected into prompts
	composed := ctx.Prompts.Compose()
	if !strings.Contains(composed, "neo-heinzel") {
		test.Error("expected facts to be injected into prompt after OnContextBuild")
	}
}

func TestMemoryComposerAddonHandleCommandStats(test *testing.T) {
	addon := NewMemoryComposerAddon(8000)
	factsLayer := NewFactsLayer()
	addon.Composer().Register(factsLayer)

	ctx := testContext()
	result := addon.HandleCommand("memory", "stats", ctx)
	if !strings.Contains(result, "facts") {
		test.Errorf("expected stats to mention 'facts', got: %s", result)
	}
}

func TestMemoryComposerRegisterAndUnregister(test *testing.T) {
	composer := NewMemoryComposer(8000)
	factsLayer := NewFactsLayer()
	recentLayer := NewRecentMessagesLayer(10)

	composer.Register(factsLayer)
	composer.Register(recentLayer)

	if len(composer.Layers()) != 2 {
		test.Errorf("expected 2 layers, got %d", len(composer.Layers()))
	}

	composer.Unregister("facts")
	if len(composer.Layers()) != 1 {
		test.Errorf("expected 1 layer after unregister, got %d", len(composer.Layers()))
	}
}

func TestSessionSummaryLayerDefaultMaxTokens(test *testing.T) {
	layer := NewSessionSummaryLayer(0)
	if layer.maxTokens != 2000 {
		test.Errorf("expected default maxTokens 2000, got %d", layer.maxTokens)
	}
}

func TestRecentMessagesLayerDefaultMaxMessages(test *testing.T) {
	layer := NewRecentMessagesLayer(0)
	if layer.maxMessages != 50 {
		test.Errorf("expected default maxMessages 50, got %d", layer.maxMessages)
	}
}
