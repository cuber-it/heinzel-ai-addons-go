// Plan mode — step-by-step task planning with approval flow.

package addons

import (
	"fmt"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// === Plan Mode (integrated) ===

type PlanState int

const (
	PlanOff       PlanState = iota
	PlanCreating
	PlanReview
	PlanExecuting
)

type PlanStep struct {
	Index  int
	Action string
	Status string
	Result string
}

type Plan struct {
	Goal    string
	Steps   []PlanStep
	State   PlanState
	Current int
}

func (addon *ReasoningAddon) handlePlanCommand(args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = strings.ToLower(parts[0])
	}

	switch sub {
	case "on":
		ctx.Set(KeyPlanMode, true)
		return "Plan mode ON."
	case "off":
		ctx.Set(KeyPlanMode, false)
		addon.activePlan = nil
		return "Plan mode OFF."
	case "", "show":
		if addon.activePlan == nil {
			mode := "OFF"
			if val, ok := ctx.Get(KeyPlanMode); ok {
				if on, ok := val.(bool); ok && on {
					mode = "ON (waiting)"
				}
			}
			return fmt.Sprintf("Plan mode: %s", mode)
		}
		return addon.formatPlan()
	case "approve", "ja", "yes", "go":
		if addon.activePlan == nil || addon.activePlan.State != PlanReview {
			return "No plan to approve."
		}
		addon.activePlan.State = PlanExecuting
		addon.activePlan.Current = 0
		return "Plan approved."
	case "reject", "nein", "no":
		addon.activePlan = nil
		return "Plan rejected."
	case "skip":
		if addon.activePlan == nil || addon.activePlan.State != PlanExecuting {
			return "No plan executing."
		}
		if addon.activePlan.Current < len(addon.activePlan.Steps) {
			addon.activePlan.Steps[addon.activePlan.Current].Status = "skipped"
			addon.activePlan.Current++
		}
		return "Step skipped."
	case "next":
		if addon.activePlan == nil || addon.activePlan.State != PlanExecuting {
			return "No plan executing."
		}
		if addon.activePlan.Current >= len(addon.activePlan.Steps) {
			return "Plan complete."
		}
		step := addon.activePlan.Steps[addon.activePlan.Current]
		step.Status = "running"
		ctx.Set(KeyExecutePlanStep, true)
		return fmt.Sprintf("Executing step %d: %s", step.Index+1, step.Action)
	case "reset":
		addon.activePlan = nil
		return "Plan reset."
	}
	return "Usage: plan [on|off|show|approve|reject|skip|next|reset]"
}

func (addon *ReasoningAddon) planIntercept(ctx *core.Context) {
	if !addon.isPlanMode(ctx) || addon.activePlan != nil {
		return
	}
	addon.activePlan = &Plan{Goal: ctx.Input, State: PlanCreating}
	ctx.Input = fmt.Sprintf(`WICHTIG: Du bist im PLAN-MODUS. Erstelle NUR einen Plan.
REGELN:
- Antworte AUSSCHLIESSLICH mit einer nummerierten Liste
- KEIN Code, KEINE Erklärungen, KEINE Beispiele
- Jede Zeile: Nummer. Konkrete Aktion
- Maximal 10 Schritte
Aufgabe: %s
Antworte NUR mit dem Plan:`, ctx.Input)
	ctx.Set(KeyPlanCreating, true)
}

func (addon *ReasoningAddon) injectPlanContext(ctx *core.Context) {
	if addon.activePlan == nil {
		return
	}
	ctx.Prompts.Set(core.LayerTurn, "plan", addon.formatPlanForLLM(), 85)
}

func (addon *ReasoningAddon) parsePlanResponse(ctx *core.Context) {
	if _, ok := ctx.Get(KeyPlanCreating); !ok {
		return
	}
	if addon.activePlan == nil || addon.activePlan.State != PlanCreating {
		return
	}

	steps := addon.simpleParseSteps(ctx.Output)

	// Fallback: extract via loop
	if len(steps) < 2 && addon.loop != nil {
		extractCtx := core.NewContext("plan-extract")
		extractCtx.SystemPrompt = "Extrahiere die Schritte als nummerierte Liste. NUR Schritte, Format: 1. Aktion"
		extractCtx.Set(KeyInternalQuery, true)
		core.InitThinking(extractCtx, nil)
		output := addon.loop.Run(extractCtx, ctx.Output)
		steps = addon.simpleParseSteps(output)
	}

	if len(steps) > 0 {
		addon.activePlan.Steps = steps
		addon.activePlan.State = PlanReview
		ctx.Output = "Plan erstellt:\n\n" + addon.formatPlan()
	}
	ctx.Set(KeyPlanCreating, nil)

	// Record step result if executing
	if addon.activePlan != nil && addon.activePlan.State == PlanExecuting {
		if addon.activePlan.Current < len(addon.activePlan.Steps) {
			addon.activePlan.Steps[addon.activePlan.Current].Status = "done"
			addon.activePlan.Steps[addon.activePlan.Current].Result = ctx.Output
			addon.activePlan.Current++
		}
	}
}

func (addon *ReasoningAddon) isPlanMode(ctx *core.Context) bool {
	val, ok := ctx.Get(KeyPlanMode)
	if !ok {
		return false
	}
	on, ok := val.(bool)
	return ok && on
}

func (addon *ReasoningAddon) simpleParseSteps(response string) []PlanStep {
	var steps []PlanStep
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' {
			dotIdx := strings.Index(line, ".")
			if dotIdx >= 0 && dotIdx < 4 {
				action := strings.TrimSpace(line[dotIdx+1:])
				if action != "" {
					steps = append(steps, PlanStep{Index: len(steps), Action: action, Status: "pending"})
				}
			}
		}
	}
	return steps
}

func (addon *ReasoningAddon) formatPlan() string {
	if addon.activePlan == nil {
		return "No active plan."
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Goal: %s", addon.activePlan.Goal))
	states := []string{"off", "creating", "review", "executing"}
	lines = append(lines, fmt.Sprintf("State: %s", states[addon.activePlan.State]))
	for _, step := range addon.activePlan.Steps {
		marker := "[ ]"
		switch step.Status {
		case "running":
			marker = "[>]"
		case "done":
			marker = "[x]"
		case "skipped":
			marker = "[-]"
		}
		lines = append(lines, fmt.Sprintf("  %s %d. %s", marker, step.Index+1, step.Action))
	}
	if addon.activePlan.State == PlanReview {
		lines = append(lines, "\nApprove: /plan approve  |  Reject: /plan reject")
	}
	return strings.Join(lines, "\n")
}

func (addon *ReasoningAddon) formatPlanForLLM() string {
	if addon.activePlan == nil {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Active plan: %s", addon.activePlan.Goal))
	for _, step := range addon.activePlan.Steps {
		lines = append(lines, fmt.Sprintf("  %d. [%s] %s", step.Index+1, step.Status, step.Action))
	}
	return strings.Join(lines, "\n")
}
