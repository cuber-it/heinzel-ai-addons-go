// WorkflowAddon — deklarative Aktionsskripte (Schank-Style).
// Engine only: parsen, ausführen, Commands, Awareness.
// Laden, Trigger-Matching, GUI — das macht die Anwendung (z.B. Assistant).

package addons

import (
	"fmt"
	"strings"
	"sync"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// Workflow defines a reusable action sequence (Schank-Style action script)
type Workflow struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Trigger     string          `yaml:"trigger"`
	Parameters  []WorkflowParam `yaml:"parameters"`
	Permissions []string        `yaml:"permissions"`
	Rules       []string        `yaml:"rules"`
	Steps       []Step          `yaml:"steps"`
	OnFail      string          `yaml:"on_fail"` // rollback | stop | continue
	Tags        []string        `yaml:"tags"`
}

// WorkflowParam defines an input parameter for a workflow
type WorkflowParam struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

// Step is one action in a workflow
type Step struct {
	Name      string `yaml:"name"`
	Action    string `yaml:"action"`
	Condition string `yaml:"condition"`
	OnFail    string `yaml:"on_fail"`
}

// WorkflowAddon is the engine — manages workflows, executes them, provides commands.
// Does NOT load from disk or match triggers — that's the application's job.
type WorkflowAddon struct {
	core.BaseAddon
	mu        sync.RWMutex
	workflows map[string]*Workflow
	loop      *core.Loop
}

func NewWorkflowAddon() *WorkflowAddon {
	return &WorkflowAddon{
		workflows: make(map[string]*Workflow),
	}
}

func (addon *WorkflowAddon) SetLoop(loop *core.Loop) { addon.loop = loop }

func (addon *WorkflowAddon) Name() string           { return "workflows" }
func (addon *WorkflowAddon) Type() core.AddonType   { return core.AddonFilter }
func (addon *WorkflowAddon) Start() error            { return nil }
func (addon *WorkflowAddon) Stop() error             { return nil }

func (addon *WorkflowAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnContextBuild}
}

func (addon *WorkflowAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "workflow", Description: "workflow engine",
			Usage: "workflow [list|show <name>|run <name>]"},
	}
}

func (addon *WorkflowAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = parts[0]
	}

	switch sub {
	case "", "list":
		return addon.ListWorkflows()
	case "show":
		if len(parts) < 2 {
			return "Usage: workflow show <name>"
		}
		return addon.ShowWorkflow(parts[1])
	case "run":
		if len(parts) < 2 {
			return "Usage: workflow run <name>"
		}
		return addon.RunWorkflow(parts[1], ctx)
	}
	return "Usage: workflow [list|show <name>|run <name>]"
}

func (addon *WorkflowAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook == core.OnContextBuild {
		addon.injectAwareness(ctx)
	}
	return core.Result{}
}

// === Public API — called by the application ===

// Register adds a workflow to the engine
func (addon *WorkflowAddon) Register(wf *Workflow) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	addon.workflows[wf.Name] = wf
}

// Unregister removes a workflow by name
func (addon *WorkflowAddon) Unregister(name string) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	delete(addon.workflows, name)
}

// Get returns a workflow by name
func (addon *WorkflowAddon) Get(name string) *Workflow {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	return addon.workflows[name]
}

// All returns all registered workflows
func (addon *WorkflowAddon) All() map[string]*Workflow {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	result := make(map[string]*Workflow, len(addon.workflows))
	for k, v := range addon.workflows {
		result[k] = v
	}
	return result
}

// Count returns the number of registered workflows
func (addon *WorkflowAddon) Count() int {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	return len(addon.workflows)
}

// MatchTrigger finds a workflow whose trigger matches the input
func (addon *WorkflowAddon) MatchTrigger(input string) *Workflow {
	lower := strings.ToLower(input)
	addon.mu.RLock()
	defer addon.mu.RUnlock()

	for _, wf := range addon.workflows {
		if wf.Trigger != "" && matchTrigger(lower, wf.Trigger) {
			return wf
		}
	}
	return nil
}

// === Engine ===

// RunWorkflow executes a workflow step by step via the LLM
func (addon *WorkflowAddon) RunWorkflow(name string, ctx *core.Context) string {
	addon.mu.RLock()
	wf, ok := addon.workflows[name]
	addon.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("Workflow %q not found.", name)
	}

	if addon.loop == nil {
		return "No loop available for workflow execution."
	}

	var results []string
	results = append(results, fmt.Sprintf("Workflow: %s (%d steps)", wf.Name, len(wf.Steps)))

	for idx, step := range wf.Steps {
		stepDesc := step.Action
		if step.Name != "" {
			stepDesc = step.Name + ": " + step.Action
		}

		stepCtx := core.NewContext("workflow")
		stepCtx.SystemPrompt = fmt.Sprintf("Du führst Schritt %d/%d eines Workflows aus. Führe die Aktion aus und berichte das Ergebnis kurz.",
			idx+1, len(wf.Steps))
		stepCtx.Set(core.KeyInternalQuery, true)
		core.InitThinking(stepCtx, nil)

		result := addon.loop.Run(stepCtx, step.Action)

		status := "done"
		if stepCtx.Error != nil {
			status = "failed"
			onFail := step.OnFail
			if onFail == "" {
				onFail = wf.OnFail
			}
			switch onFail {
			case "stop":
				results = append(results, fmt.Sprintf("  %d. [FAILED] %s — %v (stopped)", idx+1, stepDesc, stepCtx.Error))
				return strings.Join(results, "\n")
			case "rollback":
				results = append(results, fmt.Sprintf("  %d. [FAILED] %s — %v (rollback needed)", idx+1, stepDesc, stepCtx.Error))
				return strings.Join(results, "\n")
			}
		}

		results = append(results, fmt.Sprintf("  %d. [%s] %s", idx+1, status, stepDesc))
		if result != "" && len(result) < 200 {
			results = append(results, fmt.Sprintf("     → %s", result))
		}
	}

	results = append(results, "Workflow complete.")
	return strings.Join(results, "\n")
}

// ListWorkflows returns a formatted list
func (addon *WorkflowAddon) ListWorkflows() string {
	addon.mu.RLock()
	defer addon.mu.RUnlock()

	if len(addon.workflows) == 0 {
		return "No workflows loaded."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Workflows (%d):", len(addon.workflows)))
	for _, wf := range addon.workflows {
		trigger := ""
		if wf.Trigger != "" {
			trigger = fmt.Sprintf(" [trigger: %s]", wf.Trigger)
		}
		lines = append(lines, fmt.Sprintf("  %-20s %s%s", wf.Name, wf.Description, trigger))
	}
	return strings.Join(lines, "\n")
}

// ShowWorkflow returns details of one workflow
func (addon *WorkflowAddon) ShowWorkflow(name string) string {
	addon.mu.RLock()
	wf, ok := addon.workflows[name]
	addon.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("Workflow %q not found.", name)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Workflow: %s", wf.Name))
	if wf.Description != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", wf.Description))
	}
	if wf.Trigger != "" {
		lines = append(lines, fmt.Sprintf("Trigger: %s", wf.Trigger))
	}
	if len(wf.Parameters) > 0 {
		lines = append(lines, "Parameters:")
		for _, p := range wf.Parameters {
			req := ""
			if p.Required {
				req = " (required)"
			}
			lines = append(lines, fmt.Sprintf("  - %s: %s%s", p.Name, p.Description, req))
		}
	}
	if len(wf.Permissions) > 0 {
		lines = append(lines, fmt.Sprintf("Permissions: %s", strings.Join(wf.Permissions, ", ")))
	}
	if len(wf.Rules) > 0 {
		lines = append(lines, "Rules:")
		for _, r := range wf.Rules {
			lines = append(lines, fmt.Sprintf("  - %s", r))
		}
	}
	if wf.OnFail != "" {
		lines = append(lines, fmt.Sprintf("On Fail: %s", wf.OnFail))
	}
	lines = append(lines, fmt.Sprintf("Steps (%d):", len(wf.Steps)))
	for idx, step := range wf.Steps {
		n := step.Name
		if n == "" {
			n = step.Action
		}
		cond := ""
		if step.Condition != "" {
			cond = fmt.Sprintf(" (if: %s)", step.Condition)
		}
		lines = append(lines, fmt.Sprintf("  %d. %s%s", idx+1, n, cond))
	}
	return strings.Join(lines, "\n")
}

func (addon *WorkflowAddon) injectAwareness(ctx *core.Context) {
	if val, ok := ctx.Get(core.KeyInternalQuery); ok {
		if internal, ok := val.(bool); ok && internal {
			return
		}
	}

	addon.mu.RLock()
	defer addon.mu.RUnlock()

	if len(addon.workflows) == 0 {
		return
	}

	var lines []string
	lines = append(lines, "Verfügbare Workflows (ausführbar mit /workflow run <name>):")
	for _, wf := range addon.workflows {
		desc := wf.Description
		if desc == "" {
			desc = wf.Name
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s", wf.Name, desc))
	}

	ctx.Prompts.Set(core.LayerTurn, "workflows", strings.Join(lines, "\n"), 40)
}

func matchTrigger(input, trigger string) bool {
	trigger = strings.ToLower(trigger)
	if strings.HasSuffix(trigger, "*") {
		prefix := strings.TrimSuffix(trigger, "*")
		return strings.HasPrefix(input, strings.TrimSpace(prefix))
	}
	return input == trigger
}
