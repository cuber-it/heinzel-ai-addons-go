package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewWorkflowAddon(test *testing.T) {
	addon := NewWorkflowAddon()
	if addon == nil {
		test.Fatal("NewWorkflowAddon returned nil")
	}
	if addon.Name() != "workflows" {
		test.Errorf("expected name 'workflows', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
	if addon.Count() != 0 {
		test.Errorf("expected 0 workflows initially, got %d", addon.Count())
	}
}

func TestRegisterAndGet(test *testing.T) {
	addon := NewWorkflowAddon()
	workflow := &Workflow{Name: "deploy", Description: "Deploy app"}

	addon.Register(workflow)

	got := addon.Get("deploy")
	if got == nil {
		test.Fatal("expected workflow, got nil")
	}
	if got.Name != "deploy" {
		test.Errorf("expected name 'deploy', got %q", got.Name)
	}
	if got.Description != "Deploy app" {
		test.Errorf("expected description 'Deploy app', got %q", got.Description)
	}
}

func TestGetNonexistent(test *testing.T) {
	addon := NewWorkflowAddon()
	got := addon.Get("nonexistent")
	if got != nil {
		test.Errorf("expected nil for nonexistent workflow, got %v", got)
	}
}

func TestUnregister(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy"})
	addon.Register(&Workflow{Name: "backup"})

	if addon.Count() != 2 {
		test.Fatalf("expected 2 workflows, got %d", addon.Count())
	}

	addon.Unregister("deploy")

	if addon.Count() != 1 {
		test.Errorf("expected 1 workflow after unregister, got %d", addon.Count())
	}
	if addon.Get("deploy") != nil {
		test.Error("expected nil after unregister")
	}
	if addon.Get("backup") == nil {
		test.Error("expected backup to still exist")
	}
}

func TestAllWorkflows(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy"})
	addon.Register(&Workflow{Name: "backup"})

	all := addon.All()
	if len(all) != 2 {
		test.Errorf("expected 2 workflows, got %d", len(all))
	}

	// Verify it's a copy (modifying returned map should not affect addon)
	delete(all, "deploy")
	if addon.Count() != 2 {
		test.Error("All() should return a copy, not the internal map")
	}
}

func TestCount(test *testing.T) {
	addon := NewWorkflowAddon()
	if addon.Count() != 0 {
		test.Errorf("expected 0, got %d", addon.Count())
	}

	addon.Register(&Workflow{Name: "a"})
	if addon.Count() != 1 {
		test.Errorf("expected 1, got %d", addon.Count())
	}

	addon.Register(&Workflow{Name: "b"})
	if addon.Count() != 2 {
		test.Errorf("expected 2, got %d", addon.Count())
	}
}

func TestMatchTriggerGlobPattern(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Trigger: "deploy*"})
	addon.Register(&Workflow{Name: "backup", Trigger: "backup now"})

	// Glob match
	match := addon.MatchTrigger("deploy production")
	if match == nil {
		test.Fatal("expected match for 'deploy production'")
	}
	if match.Name != "deploy" {
		test.Errorf("expected 'deploy', got %q", match.Name)
	}

	// Exact match
	match = addon.MatchTrigger("backup now")
	if match == nil {
		test.Fatal("expected match for 'backup now'")
	}
	if match.Name != "backup" {
		test.Errorf("expected 'backup', got %q", match.Name)
	}

	// Case insensitive
	match = addon.MatchTrigger("DEPLOY staging")
	if match == nil {
		test.Fatal("expected case-insensitive match")
	}
}

func TestMatchTriggerNoMatch(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Trigger: "deploy*"})

	match := addon.MatchTrigger("run tests")
	if match != nil {
		test.Errorf("expected nil for no match, got %v", match.Name)
	}
}

func TestMatchTriggerEmptyTrigger(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "manual", Trigger: ""})

	match := addon.MatchTrigger("anything")
	if match != nil {
		test.Error("workflow with empty trigger should not match")
	}
}

func TestListWorkflowsEmpty(test *testing.T) {
	addon := NewWorkflowAddon()
	result := addon.ListWorkflows()
	if !strings.Contains(result, "No workflows") {
		test.Errorf("expected 'No workflows' message, got %q", result)
	}
}

func TestListWorkflowsFormatted(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Description: "Deploy app", Trigger: "deploy*"})
	addon.Register(&Workflow{Name: "backup", Description: "Backup data"})

	result := addon.ListWorkflows()
	if !strings.Contains(result, "Workflows (2)") {
		test.Errorf("expected 'Workflows (2)' header, got %q", result)
	}
	if !strings.Contains(result, "deploy") {
		test.Error("expected 'deploy' in listing")
	}
	if !strings.Contains(result, "backup") {
		test.Error("expected 'backup' in listing")
	}
	if !strings.Contains(result, "[trigger: deploy*]") {
		test.Error("expected trigger shown for deploy workflow")
	}
}

func TestShowWorkflow(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{
		Name:        "deploy",
		Description: "Deploy the application",
		Trigger:     "deploy*",
		Parameters: []WorkflowParam{
			{Name: "env", Description: "Target environment", Required: true},
			{Name: "dry-run", Description: "Dry run mode", Default: "false"},
		},
		Rules: []string{"must pass tests", "require approval"},
		Steps: []Step{
			{Name: "test", Action: "run tests"},
			{Name: "build", Action: "build app"},
		},
		OnFail: "stop",
	})

	result := addon.ShowWorkflow("deploy")
	if !strings.Contains(result, "Workflow: deploy") {
		test.Error("expected workflow name in output")
	}
	if !strings.Contains(result, "Deploy the application") {
		test.Error("expected description in output")
	}
	if !strings.Contains(result, "Parameters:") {
		test.Error("expected parameters section")
	}
	if !strings.Contains(result, "env") && !strings.Contains(result, "(required)") {
		test.Error("expected required parameter shown")
	}
	if !strings.Contains(result, "Rules:") {
		test.Error("expected rules section")
	}
	if !strings.Contains(result, "must pass tests") {
		test.Error("expected rule in output")
	}
	if !strings.Contains(result, "Steps (2)") {
		test.Error("expected steps count")
	}
	if !strings.Contains(result, "On Fail: stop") {
		test.Error("expected on_fail in output")
	}
}

func TestShowWorkflowNotFound(test *testing.T) {
	addon := NewWorkflowAddon()
	result := addon.ShowWorkflow("nonexistent")
	if !strings.Contains(result, "not found") {
		test.Errorf("expected 'not found' message, got %q", result)
	}
}

func TestHandleCommandList(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Description: "Deploy"})
	ctx := testContext()

	result := addon.HandleCommand("workflow", "list", ctx)
	if !strings.Contains(result, "deploy") {
		test.Errorf("expected deploy in list output, got %q", result)
	}

	// Empty args defaults to list
	result = addon.HandleCommand("workflow", "", ctx)
	if !strings.Contains(result, "deploy") {
		test.Errorf("expected deploy in default list output, got %q", result)
	}
}

func TestHandleCommandShow(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Description: "Deploy app"})
	ctx := testContext()

	result := addon.HandleCommand("workflow", "show deploy", ctx)
	if !strings.Contains(result, "Workflow: deploy") {
		test.Errorf("expected workflow details, got %q", result)
	}

	// Missing name
	result = addon.HandleCommand("workflow", "show", ctx)
	if !strings.Contains(result, "Usage") {
		test.Errorf("expected usage message for missing name, got %q", result)
	}
}

func TestHandleCommandUnknownSubcommand(test *testing.T) {
	addon := NewWorkflowAddon()
	ctx := testContext()

	result := addon.HandleCommand("workflow", "unknown", ctx)
	if !strings.Contains(result, "Usage") {
		test.Errorf("expected usage for unknown subcommand, got %q", result)
	}
}

func TestAwarenessInjection(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Description: "Deploy app"})

	ctx := testContext()
	addon.injectAwareness(ctx)

	composed := ctx.Prompts.Compose()
	if !strings.Contains(composed, "deploy") {
		test.Error("expected workflow awareness in prompts")
	}
	if !strings.Contains(composed, "/workflow run") {
		test.Error("expected usage hint in awareness")
	}
}

func TestAwarenessInjectionEmpty(test *testing.T) {
	addon := NewWorkflowAddon()
	ctx := testContext()

	addon.injectAwareness(ctx)

	composed := ctx.Prompts.Compose()
	if strings.Contains(composed, "Workflow") {
		test.Error("should not inject awareness when no workflows registered")
	}
}

func TestAwarenessInjectionSkipsInternal(test *testing.T) {
	addon := NewWorkflowAddon()
	addon.Register(&Workflow{Name: "deploy", Description: "Deploy"})

	ctx := testContext()
	ctx.Set(core.KeyInternalQuery, true)

	addon.injectAwareness(ctx)

	composed := ctx.Prompts.Compose()
	if strings.Contains(composed, "deploy") {
		test.Error("should not inject awareness for internal queries")
	}
}
