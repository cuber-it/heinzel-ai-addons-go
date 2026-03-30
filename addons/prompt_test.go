package addons

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewPromptAddon(test *testing.T) {
	addon := NewPromptAddon("Du bist ein Heinzel.", nil, nil)
	if addon == nil {
		test.Fatal("NewPromptAddon returned nil")
	}
	if addon.Name() != "prompt" {
		test.Errorf("expected name 'prompt', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
}

func TestPromptAddonHooks(test *testing.T) {
	addon := NewPromptAddon("test", nil, nil)
	hooks := addon.Hooks()
	if len(hooks) != 2 {
		test.Fatalf("expected 2 hooks, got %d", len(hooks))
	}
	found := map[core.HookPoint]bool{}
	for _, hook := range hooks {
		found[hook] = true
	}
	if !found[core.OnSessionStart] {
		test.Error("expected OnSessionStart hook")
	}
	if !found[core.OnContextBuild] {
		test.Error("expected OnContextBuild hook")
	}
}

func TestPromptAddonOnSessionStartSetsSystemPrompt(test *testing.T) {
	systemPrompt := "Du bist ein hilfreicher Assistent."
	addon := NewPromptAddon(systemPrompt, nil, nil)
	ctx := testContext()

	addon.Handle(core.OnSessionStart, ctx)

	composed := ctx.Prompts.Compose()
	if !strings.Contains(composed, systemPrompt) {
		test.Errorf("expected system prompt in composed output, got: %s", composed)
	}
}

func TestPromptAddonOnContextBuildAddsTurnContent(test *testing.T) {
	addon := NewPromptAddon("system", nil, nil)
	ctx := testContext()

	addon.Handle(core.OnContextBuild, ctx)

	blocks := ctx.Prompts.Blocks()
	foundDate := false
	for _, block := range blocks {
		if strings.Contains(block.Content, "Current date:") {
			foundDate = true
		}
	}
	if !foundDate {
		test.Error("expected turn content to contain current date")
	}
}

func TestPromptAddonOnContextBuildStrategyHint(test *testing.T) {
	addon := NewPromptAddon("system", nil, nil)
	ctx := testContext()
	ctx.Set(KeyStrategy, StrategyReAct)

	addon.Handle(core.OnContextBuild, ctx)

	blocks := ctx.Prompts.Blocks()
	foundHint := false
	for _, block := range blocks {
		if strings.Contains(block.Content, "Use the available tools") {
			foundHint = true
		}
	}
	if !foundHint {
		test.Error("expected ReAct strategy hint in turn content")
	}
}

func TestPromptAddonBuildAwarenessIncludesProvider(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewPromptAddon("system", dispatcher, nil)
	ctx := testContext()

	awareness := addon.buildAwareness(ctx)
	if !strings.Contains(awareness, "Dein aktueller Zustand") {
		test.Errorf("expected awareness header, got: %s", awareness)
	}
	if !strings.Contains(awareness, "Konversation:") {
		test.Errorf("expected conversation count in awareness, got: %s", awareness)
	}
}

func TestPromptAddonBuildAwarenessNilDispatcher(test *testing.T) {
	addon := NewPromptAddon("system", nil, nil)
	ctx := testContext()

	awareness := addon.buildAwareness(ctx)
	if awareness != "" {
		test.Errorf("expected empty awareness with nil dispatcher, got: %s", awareness)
	}
}

func TestPromptAddonSkillLoadingFromDir(test *testing.T) {
	skillDir := test.TempDir()

	skillYAML := `name: test-skill
description: A test skill for unit testing
output:
  format: markdown
  language: go
quality:
  - Code compiles
  - Tests pass
review:
  - Check error handling
`
	err := os.WriteFile(filepath.Join(skillDir, "test-skill.yaml"), []byte(skillYAML), 0644)
	if err != nil {
		test.Fatalf("failed to write skill YAML: %v", err)
	}

	addon := NewPromptAddon("system", nil, []string{skillDir})

	names := addon.skills.Names()
	if len(names) != 1 {
		test.Fatalf("expected 1 skill loaded, got %d", len(names))
	}

	skill := addon.skills.Get("test-skill")
	if skill == nil {
		test.Fatal("expected skill 'test-skill' to exist")
	}
	if skill.Description != "A test skill for unit testing" {
		test.Errorf("unexpected description: %s", skill.Description)
	}
}

func TestPromptAddonSkillLoadingIgnoresNonYAML(test *testing.T) {
	skillDir := test.TempDir()

	os.WriteFile(filepath.Join(skillDir, "readme.txt"), []byte("not a skill"), 0644)
	os.WriteFile(filepath.Join(skillDir, "valid.yml"), []byte("name: yml-skill\ndescription: test\n"), 0644)

	addon := NewPromptAddon("system", nil, []string{skillDir})

	names := addon.skills.Names()
	if len(names) != 1 {
		test.Errorf("expected 1 skill (only .yml), got %d: %v", len(names), names)
	}
}

func TestPromptAddonHandleCommandPromptShow(test *testing.T) {
	addon := NewPromptAddon("My system prompt", nil, nil)
	ctx := testContext()

	// Set system prompt via hook first
	addon.Handle(core.OnSessionStart, ctx)

	result := addon.HandleCommand("prompt", "show", ctx)
	if !strings.Contains(result, "Prompt layers") {
		test.Errorf("expected 'Prompt layers' in show output, got: %s", result)
	}
	if !strings.Contains(result, "system") {
		test.Errorf("expected 'system' layer in show output, got: %s", result)
	}
}

func TestPromptAddonHandleCommandPromptShowEmpty(test *testing.T) {
	addon := NewPromptAddon("", nil, nil)
	ctx := testContext()

	result := addon.HandleCommand("prompt", "show", ctx)
	if result != "No prompt blocks set." {
		test.Errorf("expected 'No prompt blocks set.', got: %s", result)
	}
}

func TestPromptAddonHandleCommandSetSystem(test *testing.T) {
	addon := NewPromptAddon("", nil, nil)
	ctx := testContext()

	result := addon.HandleCommand("prompt", "system New system prompt", ctx)
	if result != "System prompt updated." {
		test.Errorf("expected update confirmation, got: %s", result)
	}
	if addon.systemPrompt != "New system prompt" {
		test.Errorf("expected systemPrompt updated, got: %s", addon.systemPrompt)
	}
}

func TestPromptAddonHandleCommandSkillList(test *testing.T) {
	addon := NewPromptAddon("", nil, nil)
	ctx := testContext()

	result := addon.HandleCommand("skill", "list", ctx)
	if result != "No skills loaded." {
		test.Errorf("expected 'No skills loaded.', got: %s", result)
	}
}
