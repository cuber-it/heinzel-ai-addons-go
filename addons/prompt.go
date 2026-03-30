// PromptAddon — prompt composition, awareness injection, and skills.

package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"

	"gopkg.in/yaml.v3"
)

// PromptAddon manages prompt layers, awareness, AND skills
// One addon for everything that shapes the LLM context
type PromptAddon struct {
	core.BaseAddon
	systemPrompt  string
	sessionPrompt string
	userPrompt    string
	dispatcher    *core.Dispatcher
	// Skills (integrated)
	skills    *SkillRegistry
	skillDirs []string
}

func NewPromptAddon(systemPrompt string, dispatcher *core.Dispatcher, skillDirs []string) *PromptAddon {
	addon := &PromptAddon{
		systemPrompt: systemPrompt,
		dispatcher:   dispatcher,
		skills:       NewSkillRegistry(),
		skillDirs:    skillDirs,
	}
	addon.loadSkills()
	return addon
}

func (addon *PromptAddon) Name() string           { return "prompt" }
func (addon *PromptAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *PromptAddon) Start() error            { return nil }
func (addon *PromptAddon) Stop() error             { return nil }

func (addon *PromptAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnSessionStart,
		core.OnContextBuild,
	}
}

func (addon *PromptAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "prompt", Description: "manage prompt layers",
			Usage: "prompt [show|system|session|user|awareness|composed]"},
		{Name: "skill", Description: "manage skills",
			Usage: "skill [list|show <name>|use <name>|clear|review]"},
	}
}

func (addon *PromptAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	if cmd == "skill" {
		return addon.handleSkillCommand(args, ctx)
	}
	parts := strings.SplitN(args, " ", 2)
	subcmd := ""
	text := ""
	if len(parts) > 0 {
		subcmd = parts[0]
	}
	if len(parts) > 1 {
		text = parts[1]
	}

	switch subcmd {
	case "", "show":
		return addon.showPrompts(ctx)
	case "system":
		if text != "" {
			addon.systemPrompt = text
			ctx.Prompts.Set(core.LayerSystem, "prompt", text, 0)
			return "System prompt updated."
		}
		return fmt.Sprintf("System: %s", addon.systemPrompt)
	case "session":
		if text != "" {
			addon.sessionPrompt = text
			ctx.Prompts.Set(core.LayerSession, "prompt", text, 0)
			return "Session prompt updated."
		}
		return fmt.Sprintf("Session: %s", addon.sessionPrompt)
	case "user":
		if text != "" {
			addon.userPrompt = text
			ctx.Prompts.Set(core.LayerUser, "prompt", text, 0)
			return "User prompt updated."
		}
		return fmt.Sprintf("User: %s", addon.userPrompt)
	case "composed":
		return ctx.Prompts.Compose()
	case "awareness":
		return addon.buildAwareness(ctx)
	default:
		return "Usage: prompt [show|system|session|user|awareness|composed] [text]"
	}
}

func (addon *PromptAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnSessionStart:
		if addon.systemPrompt != "" {
			ctx.Prompts.Set(core.LayerSystem, "prompt", addon.systemPrompt, 0)
		}
		if addon.sessionPrompt != "" {
			ctx.Prompts.Set(core.LayerSession, "prompt", addon.sessionPrompt, 0)
		}
		if addon.userPrompt != "" {
			ctx.Prompts.Set(core.LayerUser, "prompt", addon.userPrompt, 0)
		}

	case core.OnContextBuild:
		// Dynamic turn content
		var turnParts []string
		turnParts = append(turnParts, fmt.Sprintf("Current date: %s", time.Now().Format("2006-01-02 15:04")))

		// Tools
		if val, ok := ctx.Get(KeyToolRegistry); ok {
			if reg, ok := val.(*core.ToolRegistry); ok && reg.Count() > 0 {
				var toolNames []string
				for _, tool := range reg.All() {
					toolNames = append(toolNames, tool.Name)
				}
				turnParts = append(turnParts, fmt.Sprintf("Available tools: %s", strings.Join(toolNames, ", ")))
			}
		}

		// Memory
		if len(ctx.MemoryResults) > 0 {
			turnParts = append(turnParts, "Memory context:")
			for key, val := range ctx.MemoryResults {
				turnParts = append(turnParts, fmt.Sprintf("  [%s]: %v", key, val))
			}
		}

		// Strategy hint — CoT and Deep are handled by multi-step reasoning in the addon.
		// Only ReAct needs a prompt hint (tool usage).
		if val, ok := ctx.Get(KeyStrategy); ok {
			if strategy, ok := val.(Strategy); ok {
				if strategy == StrategyReAct {
					turnParts = append(turnParts, "Use the available tools to accomplish this task.")
				}
			}
		}

		if len(turnParts) > 0 {
			ctx.Prompts.Set(core.LayerTurn, "prompt", strings.Join(turnParts, "\n"), 0)
		}

		// Active skill injection
		addon.injectActiveSkill(ctx)

		// Awareness — actual system state
		awareness := addon.buildAwareness(ctx)
		if awareness != "" {
			ctx.Prompts.Set(core.LayerTurn, "awareness", awareness, 100)
		}
	}

	return core.Result{}
}

// buildAwareness collects actual system capabilities
func (addon *PromptAddon) buildAwareness(ctx *core.Context) string {
	if addon.dispatcher == nil {
		return ""
	}

	var sections []string
	sections = append(sections, "=== Dein aktueller Zustand ===")
	sections = append(sections, "Behaupte NIEMALS Fähigkeiten die hier nicht aufgeführt sind.")

	// Provider
	caps := addon.dispatcher.GetProviderCapabilities()
	if caps != nil {
		var features []string
		if caps.Streaming {
			features = append(features, "streaming")
		}
		if caps.Vision {
			features = append(features, "bilder")
		}
		if caps.ToolUse {
			features = append(features, "tool-use")
		}
		sections = append(sections, fmt.Sprintf("LLM: %s (%s) [%s]",
			caps.ProviderName, caps.ModelName, strings.Join(features, ", ")))
	}

	// Memory
	var memLines []string
	for _, name := range addon.dispatcher.ListAddons() {
		a, ok := addon.dispatcher.GetAddon(name)
		if !ok {
			continue
		}
		if mcp, ok := a.(core.MemoryCapabilityProvider); ok {
			mc := mcp.MemoryCapabilities()
			memLines = append(memLines, fmt.Sprintf("- %s: %s", mc.Name, mc.Description))
		}
	}
	if len(memLines) > 0 {
		sections = append(sections, "Gedächtnis:\n"+strings.Join(memLines, "\n"))
	} else {
		sections = append(sections, "Gedächtnis: Kein persistentes Gedächtnis aktiv.")
	}

	// Tools
	if val, ok := ctx.Get(KeyToolRegistry); ok {
		if reg, ok := val.(*core.ToolRegistry); ok && reg.Count() > 0 {
			sections = append(sections, fmt.Sprintf("Tools: %d verfügbar", reg.Count()))
		}
	}

	// Web
	for _, name := range addon.dispatcher.ListAddons() {
		if name == "websearch" {
			sections = append(sections, "Web: verfügbar")
			break
		}
	}

	// Budget
	if a, ok := addon.dispatcher.GetAddon("costguard"); ok {
		if bp, ok := a.(core.BudgetProvider); ok {
			pct, _, _ := bp.BudgetStatus()
			if pct > 80 {
				sections = append(sections, fmt.Sprintf("Budget: %.0f%% — bald erschöpft!", pct))
			}
		}
	}

	// Active skill
	if val, ok := ctx.Get(KeyActiveSkill); ok {
		if s, ok := val.(string); ok && s != "" {
			sections = append(sections, fmt.Sprintf("Skill: %s", s))
		}
	}

	sections = append(sections, fmt.Sprintf("Konversation: %d Nachrichten", len(ctx.Messages)))
	return strings.Join(sections, "\n")
}

// === Skills ===

func (addon *PromptAddon) handleSkillCommand(args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = parts[0]
	}

	switch sub {
	case "", "list":
		names := addon.skills.Names()
		if len(names) == 0 {
			return "No skills loaded."
		}
		var lines []string
		active := ""
		if val, ok := ctx.Get(KeyActiveSkill); ok {
			if s, ok := val.(string); ok {
				active = s
			}
		}
		header := "Skills:"
		if active != "" {
			header = fmt.Sprintf("Skills (active: %s):", active)
		}
		for _, name := range names {
			skill := addon.skills.Get(name)
			lines = append(lines, fmt.Sprintf("  %-20s %s", name, skill.Description))
		}
		return header + "\n" + strings.Join(lines, "\n")
	case "show":
		if len(parts) < 2 {
			return "Usage: skill show <name>"
		}
		skill := addon.skills.Get(parts[1])
		if skill == nil {
			return fmt.Sprintf("Skill %q not found.", parts[1])
		}
		return skill.FormatAsPrompt()
	case "use":
		if len(parts) < 2 {
			return "Usage: skill use <name>"
		}
		skill := addon.skills.Get(parts[1])
		if skill == nil {
			return fmt.Sprintf("Skill %q not found.", parts[1])
		}
		ctx.Set(KeyActiveSkill, parts[1])
		return fmt.Sprintf("Skill activated: %s", skill.Name)
	case "clear":
		ctx.Set(KeyActiveSkill, nil)
		return "Skill deactivated."
	case "review":
		output, ok := ctx.Get(KeyLastOutputForReview)
		if !ok {
			return "No output to review."
		}
		skillName, _ := ctx.Get(KeyActiveSkill)
		name, _ := skillName.(string)
		if name == "" {
			return "No skill active."
		}
		skill := addon.skills.Get(name)
		if skill == nil {
			return "Skill not found."
		}
		return skill.FormatReviewPrompt(output.(string))
	}
	return "Usage: skill [list|show <name>|use <name>|clear|review]"
}

func (addon *PromptAddon) loadSkills() {
	for _, dir := range addon.skillDirs {
		if dir == "" {
			continue
		}
		addon.loadSkillsFromDir(dir)
	}
}

func (addon *PromptAddon) loadSkillsFromDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var skill Skill
		if yaml.Unmarshal(data, &skill) == nil {
			addon.skills.Register(&skill)
		}
	}
}

func (addon *PromptAddon) injectActiveSkill(ctx *core.Context) {
	if val, ok := ctx.Get(KeyActiveSkill); ok {
		if name, ok := val.(string); ok && name != "" {
			skill := addon.skills.Get(name)
			if skill != nil {
				ctx.Prompts.Set(core.LayerTurn, "skill", skill.FormatAsPrompt(), 90)
			}
		}
	}
	// Store output for /skill review
	if ctx.Output != "" {
		ctx.Set(KeyLastOutputForReview, ctx.Output)
	}
}

func (addon *PromptAddon) showPrompts(ctx *core.Context) string {
	var lines []string
	for _, block := range ctx.Prompts.Blocks() {
		preview := block.Content
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		lines = append(lines, fmt.Sprintf("  [%s] (%s, pri:%d) %s",
			block.Layer, block.Source, block.Priority, preview))
	}
	if len(lines) == 0 {
		return "No prompt blocks set."
	}
	return "Prompt layers:\n" + strings.Join(lines, "\n")
}

// thinkTagInstruction wraps a reasoning instruction with thinking marker guidance.
// Uses ### markers (works with all models) parsed by the provider streaming handler.
func thinkTagInstruction(instruction string) string {
	return fmt.Sprintf(`%s

YOU MUST FORMAT YOUR RESPONSE AS FOLLOWS — NO EXCEPTIONS:

<think>
[Write your thinking here: observations, considerations, reasoning, conclusions]
</think>

[Write your final answer here, after the closing tag]

START your response with <think> — do NOT skip it. This is mandatory.`, instruction)
}
