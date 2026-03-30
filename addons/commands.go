// CommandAddon — universal slash-command handler for all IO bridges.

package addons

import (
	"fmt"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// universalHandler processes a command and returns (response, handled).
type universalHandler func(args string, ctx *core.Context) (string, bool)

// CommandAddon intercepts /-commands on OnInput before they reach the LLM
// Works for ALL IOBridges: CLI, GUI, Mattermost
// CLI-specific commands (history, bash) stay in CLI; universal commands live here
type CommandAddon struct {
	core.BaseAddon
	dispatcher *core.Dispatcher
	lastInput  string
	lastOutput string
	turnCount  int
	handlers   map[string]universalHandler
}

func NewCommandAddon(dispatcher *core.Dispatcher) *CommandAddon {
	addon := &CommandAddon{dispatcher: dispatcher}
	addon.initHandlers()
	return addon
}

func (addon *CommandAddon) Name() string           { return "commands" }
func (addon *CommandAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *CommandAddon) Start() error            { return nil }
func (addon *CommandAddon) Stop() error             { return nil }

func (addon *CommandAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnInput,
		core.OnOutput,
	}
}

func (addon *CommandAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnInput:
		input := strings.TrimSpace(ctx.Input)

		// Track for /retry
		if !strings.HasPrefix(input, "/") {
			addon.lastInput = input
		}

		// Detect / commands
		if strings.HasPrefix(input, "/") {
			result, handled := addon.handleCommand(input, ctx)
			if handled {
				ctx.Output = result
				ctx.Halt = true
				return core.Result{Halt: true}
			}
		}

		// Detect ! commands (also for GUI/Mattermost with ! prefix)
		if strings.HasPrefix(input, "!") {
			// Convert to / for addon dispatch
			converted := "/" + strings.TrimPrefix(input, "!")
			result, handled := addon.handleCommand(converted, ctx)
			if handled {
				ctx.Output = result
				ctx.Halt = true
				return core.Result{Halt: true}
			}
		}

	case core.OnOutput:
		addon.lastOutput = ctx.Output
		addon.turnCount++
	}

	return core.Result{}
}

func (addon *CommandAddon) initHandlers() {
	addon.handlers = map[string]universalHandler{
		"/help":       func(_ string, _ *core.Context) (string, bool) { return addon.showHelp(), true },
		"/status":     func(_ string, ctx *core.Context) (string, bool) { return addon.showStatus(ctx), true },
		"/clear":      addon.cmdClear,
		"/restart":    addon.cmdRestart,
		"/retry":      addon.cmdRetry,
		"/undo":       addon.cmdUndo,
		"/btw":        addon.cmdBtw,
		"/diff":       func(_ string, ctx *core.Context) (string, bool) { return addon.showDiff(ctx), true },
		"/export":     func(_ string, ctx *core.Context) (string, bool) { return addon.exportSession(ctx), true },
		"/history":    addon.cmdHistory,
		"/name":       addon.cmdName,
		"/addons":     addon.cmdAddons,
		"/hooks":      addon.cmdHooks,
		"/thinking":   addon.cmdThinking,
		"/strategy":   addon.cmdStrategy,
		"/context":    addon.cmdContext,
		"/resume":     addon.cmdResume,
		"/checkpoint": addon.cmdCheckpoint,
		"/rewind":     addon.cmdRewind,
		"/import":     addon.cmdImport,
	}
}

func (addon *CommandAddon) handleCommand(input string, ctx *core.Context) (string, bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", false
	}
	command := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	if handler, ok := addon.handlers[command]; ok {
		return handler(args, ctx)
	}

	// Try addon-registered commands
	cmdName := strings.TrimPrefix(command, "/")
	response, handled := addon.dispatcher.DispatchCommand(cmdName, args, ctx)
	if handled {
		return response, true
	}
	return "", false
}

func (addon *CommandAddon) cmdClear(_ string, ctx *core.Context) (string, bool) {
	ctx.Messages = nil
	ctx.Output = ""
	ctx.MemoryResults = make(map[string]interface{})
	return "Konversation gelöscht.", true
}

func (addon *CommandAddon) cmdRestart(_ string, ctx *core.Context) (string, bool) {
	exportPath := addon.exportSession(ctx)
	ctx.Messages = nil
	ctx.Output = ""
	ctx.MemoryResults = make(map[string]interface{})
	ctx.Prompts.ClearLayer(core.LayerTurn)
	ctx.Prompts.ClearLayer(core.LayerSession)
	if response, handled := addon.dispatcher.DispatchCommand("costs", "reset", ctx); handled {
		_ = response
	}
	addon.turnCount = 0
	msg := "Session neu gestartet."
	if exportPath != "" {
		msg += " Gespeichert: " + exportPath
	}
	return msg, true
}

func (addon *CommandAddon) cmdRetry(_ string, ctx *core.Context) (string, bool) {
	if addon.lastInput == "" {
		return "Nichts zum Wiederholen.", true
	}
	ctx.Input = addon.lastInput
	ctx.Halt = false
	return "", false // not handled — pass through with modified input
}

func (addon *CommandAddon) cmdUndo(_ string, ctx *core.Context) (string, bool) {
	if len(ctx.Messages) >= 2 {
		ctx.Messages = ctx.Messages[:len(ctx.Messages)-2]
		addon.turnCount--
		return fmt.Sprintf("Letzter Turn entfernt. (%d Messages verbleibend)", len(ctx.Messages)), true
	}
	return "Nichts zum Rückgängigmachen.", true
}

func (addon *CommandAddon) cmdBtw(args string, ctx *core.Context) (string, bool) {
	if args == "" {
		return "Usage: /btw <Frage>", true
	}
	sideCtx := core.NewContext("btw")
	sideCtx.SystemPrompt = "Antworte kurz und präzise. Maximal 3 Sätze."
	sideCtx.OnToken = ctx.OnToken
	core.InitThinking(sideCtx, nil)
	ctx.Set(KeyBtwQuestion, args)
	return "", false // IOBridge handles this
}

func (addon *CommandAddon) cmdHistory(args string, ctx *core.Context) (string, bool) {
	recallArgs := "last"
	if args != "" {
		recallArgs = "last " + args
	}
	response, handled := addon.dispatcher.DispatchCommand("recall", recallArgs, ctx)
	if handled {
		return response, true
	}
	return "Kein Transcript-Addon geladen.", true
}

func (addon *CommandAddon) cmdName(args string, ctx *core.Context) (string, bool) {
	if args != "" {
		ctx.Set(KeySessionName, args)
		return fmt.Sprintf("Session: %s", args), true
	}
	if name, ok := ctx.Get(KeySessionName); ok {
		return fmt.Sprintf("Session: %s", name), true
	}
	return fmt.Sprintf("Session: %s (unnamed)", ctx.SessionID), true
}

func (addon *CommandAddon) cmdAddons(_ string, _ *core.Context) (string, bool) {
	var lines []string
	for _, name := range addon.dispatcher.ListAddons() {
		addonInfo, _ := addon.dispatcher.GetAddon(name)
		lines = append(lines, fmt.Sprintf("  %-20s [%s]", name, addonInfo.Type()))
	}
	return strings.Join(lines, "\n"), true
}

func (addon *CommandAddon) cmdHooks(_ string, _ *core.Context) (string, bool) {
	var lines []string
	for hook := core.HookPoint(0); hook < core.HookCount; hook++ {
		subs := addon.dispatcher.HookSubscribers(hook)
		if len(subs) > 0 {
			lines = append(lines, fmt.Sprintf("  %-25s %s", hook, strings.Join(subs, ", ")))
		}
	}
	return strings.Join(lines, "\n"), true
}

func (addon *CommandAddon) cmdThinking(args string, ctx *core.Context) (string, bool) {
	current := false
	if val, ok := ctx.Get(KeyThinkingVisible); ok {
		if boolVal, ok := val.(bool); ok {
			current = boolVal
		}
	}
	if args == "on" {
		current = true
	} else if args == "off" {
		current = false
	} else {
		current = !current
	}
	ctx.Set(KeyThinkingVisible, current)
	state := "off"
	if current {
		state = "on"
	}
	return fmt.Sprintf("Thinking display: %s", state), true
}

func (addon *CommandAddon) cmdStrategy(args string, ctx *core.Context) (string, bool) {
	if args != "" {
		switch strings.ToLower(args) {
		case "passthrough", "pass":
			ctx.Set(KeyStrategyOverride, StrategyPassthrough)
		case "cot", "chain":
			ctx.Set(KeyStrategyOverride, StrategyChainOfThought)
		case "deep":
			ctx.Set(KeyStrategyOverride, StrategyDeepReasoning)
		case "react":
			ctx.Set(KeyStrategyOverride, StrategyReAct)
		case "auto":
			ctx.Set(KeyStrategyOverride, nil)
		default:
			return fmt.Sprintf("Unbekannt: %s (passthrough|cot|deep|react|auto)", args), true
		}
	}
	if val, ok := ctx.Get(KeyStrategyOverride); ok && val != nil {
		return fmt.Sprintf("Strategy: %s", val), true
	}
	return "Strategy: auto", true
}

func (addon *CommandAddon) cmdContext(_ string, ctx *core.Context) (string, bool) {
	var lines []string
	ctx.RLock()
	for key, val := range ctx.State {
		valStr := fmt.Sprintf("%v", val)
		if len(valStr) > 60 {
			valStr = valStr[:60] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %-20s %s", key, valStr))
	}
	ctx.RUnlock()
	if len(lines) == 0 {
		return "(leer)", true
	}
	return strings.Join(lines, "\n"), true
}

func (addon *CommandAddon) cmdResume(args string, ctx *core.Context) (string, bool) {
	if args == "list" {
		return addon.listSessions(), true
	}
	if args != "" {
		return addon.resumeByID(args, ctx), true
	}
	return addon.resumeLatest(ctx), true
}

func (addon *CommandAddon) cmdCheckpoint(_ string, ctx *core.Context) (string, bool) {
	addon.saveCheckpoint(ctx)
	return fmt.Sprintf("Checkpoint gespeichert (Turn %d, %d Messages)", addon.turnCount, len(ctx.Messages)), true
}

func (addon *CommandAddon) cmdRewind(args string, ctx *core.Context) (string, bool) {
	if args != "" {
		var target int
		fmt.Sscanf(args, "%d", &target)
		return addon.rewindTo(target, ctx), true
	}
	return addon.rewindLast(ctx), true
}

func (addon *CommandAddon) cmdImport(args string, ctx *core.Context) (string, bool) {
	if args == "" {
		return "Usage: /import <path|glob>", true
	}
	return addon.importFiles(args, ctx), true
}

func (addon *CommandAddon) showHelp() string {
	var lines []string
	lines = append(lines, "Commands (/ oder ! als Prefix):")
	lines = append(lines, "")
	lines = append(lines, "  help                  — this help")
	lines = append(lines, "  quit                  — exit")
	lines = append(lines, "  retry                 — repeat last turn")
	lines = append(lines, "  undo                  — remove last turn from context")
	lines = append(lines, "  btw <frage>           — side question (isolated)")
	lines = append(lines, "  diff                  — difference between last two outputs")
	lines = append(lines, "  import <path|glob>    — load files into context")
	lines = append(lines, "  checkpoint            — save current state")
	lines = append(lines, "  rewind [N]            — rewind to checkpoint")
	lines = append(lines, "  history [N]           — show last N turns")
	lines = append(lines, "  resume [list|id]      — load session")
	lines = append(lines, "  export                — save session as Markdown")
	lines = append(lines, "  name <name>           — name this session")
	lines = append(lines, "  addons                — list registered addons")
	lines = append(lines, "  hooks                 — show hook subscriptions")
	lines = append(lines, "  thinking [on|off]     — toggle thinking display")
	lines = append(lines, "  strategy [name]       — force strategy")
	lines = append(lines, "  status                — session info")
	lines = append(lines, "  context               — show internal state")
	lines = append(lines, "  clear                 — clear conversation")

	addonCmds := addon.dispatcher.AllCommands()
	if len(addonCmds) > 0 {
		lines = append(lines, "")
		for _, cmd := range addonCmds {
			lines = append(lines, fmt.Sprintf("  %-22s — %s", cmd.Name, cmd.Description))
		}
	}

	return strings.Join(lines, "\n")
}
