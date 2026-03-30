// CLI command handlers — dispatch map and per-command methods.

package addons

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const maxFileUploadBytes = 10 * 1024 * 1024 // 10 MB file upload limit

func (cli *CLIBridge) initCommandHandlers() {
	cli.commandHandlers = map[string]cliHandler{
		"/quit": func(_ []string, _ *core.Context, _ *core.Loop) bool { return true },
		"/bye":  func(_ []string, _ *core.Context, _ *core.Loop) bool { return true },
		"/exit": func(_ []string, _ *core.Context, _ *core.Loop) bool { return true },

		"/help":       cli.handleHelpCmd,
		"/addons":     cli.handleAddonsCmd,
		"/hooks":      cli.handleHooksCmd,
		"/history":    cli.handleHistoryCmd,
		"/thinking":   cli.handleThinkingCmd,
		"/strategy":   cli.handleStrategyCmd,
		"/status":     cli.handleStatusCmd,
		"/context":    cli.handleContextCmd,
		"/clear":      cli.handleClearCmd,
		"/retry":      cli.handleRetryCmd,
		"/undo":       cli.handleUndoCmd,
		"/btw":        cli.handleBtwCmd,
		"/export":     cli.handleExportCmd,
		"/resume":     cli.handleResumeCmd,
		"/diff":       cli.handleDiffCmd,
		"/import":     cli.handleImportCmd,
		"/checkpoint": cli.handleCheckpointCmd,
		"/rewind":     cli.handleRewindCmd,
		"/name":       cli.handleNameCmd,
		"/speak":      cli.handleSpeakCmd,
	}
}

func (cli *CLIBridge) handleCommand(cmd string, loop *core.Loop, ctx *core.Context) bool {
	parts := strings.Fields(cmd)
	command := strings.ToLower(parts[0])

	if handler, ok := cli.commandHandlers[command]; ok {
		return handler(parts, ctx, loop)
	}

	cmdName := strings.TrimPrefix(command, "/")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}
	response, handled := loop.Dispatcher.DispatchCommand(cmdName, args, ctx)
	if handled {
		if response != "" {
			fmt.Println(response)
		}
	} else {
		fmt.Printf("  %sUnknown command: %s%s\n", colorRed, command, colorReset)
	}
	return false
}

func (cli *CLIBridge) handleHelpCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	fmt.Printf("%sSystem Commands:%s\n", colorBold, colorReset)
	fmt.Println("  /help                 — this help")
	fmt.Println("  /quit                 — exit")
	fmt.Println("  /retry                — repeat last turn")
	fmt.Println("  /undo                 — remove last turn from context")
	fmt.Println("  /btw <frage>          — side question (isolated, no context pollution)")
	fmt.Println("  /diff                 — show difference between last two outputs")
	fmt.Println("  /import <path|glob>   — load files into context")
	fmt.Println("  /checkpoint           — save current state for rewind")
	fmt.Println("  /rewind [N]           — rewind to checkpoint (or last)")
	fmt.Println("  /resume [list|id]     — load session")
	fmt.Println("  /export               — save session as Markdown")
	fmt.Println("  /name <name>          — name this session")
	fmt.Println("  /addons               — list registered addons")
	fmt.Println("  /hooks                — show hook subscriptions")
	fmt.Println("  /history [N]          — show last N inputs (default 20)")
	fmt.Println("  /history save|clear   — save/clear history")
	fmt.Println("  /thinking [on|off]    — toggle thinking display")
	fmt.Println("  /strategy [name]      — force strategy (passthrough|cot|deep|react)")
	fmt.Println("  /status               — show session status")
	fmt.Println("  /context              — show context state")
	fmt.Println("  /clear                — clear conversation")
	fmt.Println("  !<command>            — execute bash command")
	fmt.Println("  Multi-line: end line with \\ to continue")
	addonCmds := loop.Dispatcher.AllCommands()
	if len(addonCmds) > 0 {
		fmt.Printf("\n%sAddon Commands:%s\n", colorBold, colorReset)
		for _, addonCmd := range addonCmds {
			fmt.Printf("  /%-20s — %s\n", addonCmd.Name, addonCmd.Description)
			if addonCmd.Usage != "" {
				fmt.Printf("  %s  %s%s\n", colorGray, addonCmd.Usage, colorReset)
			}
		}
	}
	return false
}

func (cli *CLIBridge) handleAddonsCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	for _, name := range loop.Dispatcher.ListAddons() {
		addon, _ := loop.Dispatcher.GetAddon(name)
		fmt.Printf("  %s%-20s%s [%s]\n", colorCyan, name, colorReset, addon.Type())
	}
	return false
}

func (cli *CLIBridge) handleHooksCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	for hook := core.HookPoint(0); hook < core.HookCount; hook++ {
		subs := loop.Dispatcher.HookSubscribers(hook)
		if len(subs) > 0 {
			fmt.Printf("  %s%-25s%s %s\n", colorYellow, hook, colorReset, strings.Join(subs, ", "))
		}
	}
	return false
}

func (cli *CLIBridge) handleHistoryCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 && parts[1] == "save" {
		cli.saveHistory()
		return false
	}
	if len(parts) > 1 && parts[1] == "clear" {
		cli.history = nil
		fmt.Printf("  %sHistory cleared.%s\n", colorGreen, colorReset)
		return false
	}
	if len(cli.history) == 0 {
		fmt.Printf("  %s(empty)%s\n", colorGray, colorReset)
	}
	start := 0
	limit := 20
	if len(parts) > 1 {
		if n, err := fmt.Sscanf(parts[1], "%d", &limit); n == 1 && err == nil {
			// ok
		}
	}
	if len(cli.history) > limit {
		start = len(cli.history) - limit
	}
	for idx := start; idx < len(cli.history); idx++ {
		fmt.Printf("  %s%3d%s  %s\n", colorGray, idx+1, colorReset, cli.history[idx])
	}
	if start > 0 {
		fmt.Printf("  %s(%d ältere nicht angezeigt, /history %d für mehr)%s\n",
			colorGray, start, len(cli.history), colorReset)
	}
	return false
}

func (cli *CLIBridge) handleThinkingCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 {
		switch strings.ToLower(parts[1]) {
		case "on":
			cli.showThinking = true
		case "off":
			cli.showThinking = false
		}
	} else {
		cli.showThinking = !cli.showThinking
	}
	state := "off"
	if cli.showThinking {
		state = "on"
	}
	fmt.Printf("  Thinking display: %s%s%s\n", colorCyan, state, colorReset)
	return false
}

func (cli *CLIBridge) handleStrategyCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 {
		switch strings.ToLower(parts[1]) {
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
			fmt.Printf("  %sUnknown strategy: %s%s\n", colorRed, parts[1], colorReset)
		}
	}
	if val, ok := ctx.Get(KeyStrategyOverride); ok && val != nil {
		fmt.Printf("  Strategy forced: %s%s%s\n", colorCyan, val, colorReset)
	} else {
		fmt.Printf("  Strategy: %sauto%s\n", colorCyan, colorReset)
	}
	return false
}

func (cli *CLIBridge) handleStatusCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	duration := time.Since(cli.sessionStart).Round(time.Second)
	fmt.Printf("  Session:  %s%s%s\n", colorCyan, ctx.SessionID, colorReset)
	fmt.Printf("  Duration: %s%s%s\n", colorCyan, duration, colorReset)
	fmt.Printf("  Turns:    %s%d%s\n", colorCyan, cli.turnCount, colorReset)
	fmt.Printf("  Messages: %s%d%s\n", colorCyan, len(ctx.Messages), colorReset)
	fmt.Printf("  Thinking: %s%v%s\n", colorCyan, cli.showThinking, colorReset)
	if val, ok := ctx.Get(KeyStrategy); ok {
		fmt.Printf("  Strategy: %s%s%s\n", colorCyan, val, colorReset)
	}
	addons := loop.Dispatcher.ListAddons()
	fmt.Printf("  Addons:   %s%d%s\n", colorCyan, len(addons), colorReset)
	return false
}

func (cli *CLIBridge) handleContextCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	ctx.RLock()
	for key, val := range ctx.State {
		fmt.Printf("  %s%-20s%s %v\n", colorYellow, key, colorReset, val)
	}
	ctx.RUnlock()
	return false
}

func (cli *CLIBridge) handleClearCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	ctx.Messages = nil
	ctx.Output = ""
	ctx.MemoryResults = make(map[string]interface{})
	cli.turnCount = 0
	fmt.Printf("  %sConversation cleared.%s\n", colorGreen, colorReset)
	return false
}

func (cli *CLIBridge) handleRetryCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if cli.lastInput == "" {
		fmt.Printf("  %sNichts zum Wiederholen.%s\n", colorGray, colorReset)
	} else {
		fmt.Printf("  %sRetry: %s%s\n", colorCyan, cli.lastInput, colorReset)
		ctx.Set(KeyOutputStreamed, nil)
		output := loop.Run(ctx, cli.lastInput)
		cli.lastOutput = output
		if output != "" {
			if _, ok := ctx.Get(KeyOutputStreamed); !ok {
				fmt.Printf("%s%s%s\n", colorBold, output, colorReset)
			}
		}
	}
	return false
}

func (cli *CLIBridge) handleUndoCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(ctx.Messages) >= 2 {
		ctx.Messages = ctx.Messages[:len(ctx.Messages)-2]
		cli.turnCount--
		fmt.Printf("  %sLetzter Turn entfernt. (%d Messages verbleibend)%s\n",
			colorGreen, len(ctx.Messages), colorReset)
	} else {
		fmt.Printf("  %sNichts zum Rückgängigmachen.%s\n", colorGray, colorReset)
	}
	return false
}

func (cli *CLIBridge) handleBtwCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) < 2 {
		fmt.Println("  Usage: /btw <Frage>")
	} else {
		question := strings.Join(parts[1:], " ")
		sideCtx := core.NewContext("btw")
		sideCtx.SystemPrompt = "Antworte kurz und präzise. Maximal 3 Sätze."
		sideCtx.OnToken = ctx.OnToken
		core.InitThinking(sideCtx, nil)
		ctx.Set(KeyOutputStreamed, nil)
		output := loop.Run(sideCtx, question)
		if output != "" {
			if _, ok := ctx.Get(KeyOutputStreamed); !ok {
				fmt.Printf("%s%s%s\n", colorBold, output, colorReset)
			}
		}
	}
	return false
}

func (cli *CLIBridge) handleExportCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	path := cli.exportSession(ctx)
	if path != "" {
		fmt.Printf("  %sExportiert: %s%s\n", colorGreen, path, colorReset)
	}
	return false
}

func (cli *CLIBridge) handleResumeCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 && parts[1] == "list" {
		cli.listSessions()
	} else if len(parts) > 1 {
		cli.resumeSessionByID(parts[1], ctx)
	} else {
		cli.resumeSession(ctx)
	}
	return false
}

func (cli *CLIBridge) handleDiffCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	cli.showDiff(ctx)
	return false
}

func (cli *CLIBridge) handleImportCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) < 2 {
		fmt.Println("  Usage: /import <path|glob|url>")
	} else {
		target := strings.Join(parts[1:], " ")
		cli.importFiles(target, ctx)
	}
	return false
}

func (cli *CLIBridge) handleCheckpointCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	cli.saveCheckpoint(ctx)
	fmt.Printf("  %sCheckpoint gespeichert (Turn %d, %d Messages)%s\n",
		colorGreen, cli.turnCount, len(ctx.Messages), colorReset)
	return false
}

func (cli *CLIBridge) handleRewindCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 {
		var target int
		fmt.Sscanf(parts[1], "%d", &target)
		cli.rewindTo(target, ctx)
	} else {
		cli.rewindLast(ctx)
	}
	return false
}

func (cli *CLIBridge) handleNameCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	if len(parts) > 1 {
		cli.sessionName = strings.Join(parts[1:], " ")
		fmt.Printf("  Session: %s%s%s\n", colorCyan, cli.sessionName, colorReset)
	} else if cli.sessionName != "" {
		fmt.Printf("  Session: %s%s%s\n", colorCyan, cli.sessionName, colorReset)
	} else {
		fmt.Printf("  Session: %s%s%s (unnamed)\n", colorCyan, ctx.SessionID, colorReset)
	}
	return false
}

func (cli *CLIBridge) showDiff(ctx *core.Context) {
	var outputs []string
	for _, msg := range ctx.Messages {
		if msg.Role == "assistant" {
			outputs = append(outputs, msg.Content)
		}
	}
	if len(outputs) < 2 {
		fmt.Printf("  %sNicht genug Turns für einen Diff.%s\n", colorGray, colorReset)
		return
	}
	prev := outputs[len(outputs)-2]
	curr := outputs[len(outputs)-1]

	prevLines := strings.Split(prev, "\n")
	currLines := strings.Split(curr, "\n")

	fmt.Printf("  %s--- Turn %d%s\n", colorRed, len(outputs)-1, colorReset)
	fmt.Printf("  %s+++ Turn %d%s\n", colorGreen, len(outputs), colorReset)

	maxLines := len(prevLines)
	if len(currLines) > maxLines {
		maxLines = len(currLines)
	}
	for idx := 0; idx < maxLines; idx++ {
		prevLine := ""
		currLine := ""
		if idx < len(prevLines) {
			prevLine = prevLines[idx]
		}
		if idx < len(currLines) {
			currLine = currLines[idx]
		}
		if prevLine != currLine {
			if prevLine != "" {
				fmt.Printf("  %s- %s%s\n", colorRed, prevLine, colorReset)
			}
			if currLine != "" {
				fmt.Printf("  %s+ %s%s\n", colorGreen, currLine, colorReset)
			}
		}
	}
}

func (cli *CLIBridge) importFiles(pattern string, ctx *core.Context) {
	if strings.HasPrefix(pattern, "http://") || strings.HasPrefix(pattern, "https://") {
		ctx.Prompts.Add(core.LayerTurn, "import:"+pattern, fmt.Sprintf("Lade URL: %s", pattern), 70)
		fmt.Printf("  %sURL in Kontext: %s%s\n", colorGreen, pattern, colorReset)
		return
	}

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
	if _, err := os.Stat(pattern); err == nil {
			matches = []string{pattern}
		} else {
			fmt.Printf("  %sKeine Dateien gefunden: %s%s\n", colorRed, pattern, colorReset)
			return
		}
	}

	total := 0
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() > maxFileUploadBytes {
			fmt.Printf("  %sÜbersprungen (zu groß): %s%s\n", colorYellow, path, colorReset)
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		block := fmt.Sprintf("[File: %s]\n```\n%s\n```", name, string(content))
		ctx.Prompts.Add(core.LayerTurn, "import:"+name, block, 60)
		total++
	}
	fmt.Printf("  %s%d Datei(en) in Kontext geladen%s\n", colorGreen, total, colorReset)
}

type checkpoint struct {
	turn     int
	messages []core.Message
	state    map[string]interface{}
}

var checkpoints []checkpoint

func (cli *CLIBridge) saveCheckpoint(ctx *core.Context) {
	msgs := make([]core.Message, len(ctx.Messages))
	copy(msgs, ctx.Messages)

	ctx.RLock()
	stateCopy := make(map[string]interface{})
	for key, val := range ctx.State {
		stateCopy[key] = val
	}
	ctx.RUnlock()

	checkpoints = append(checkpoints, checkpoint{
		turn:     cli.turnCount,
		messages: msgs,
		state:    stateCopy,
	})
}

func (cli *CLIBridge) rewindTo(target int, ctx *core.Context) {
	for idx := len(checkpoints) - 1; idx >= 0; idx-- {
		if checkpoints[idx].turn <= target {
			cp := checkpoints[idx]
			ctx.Messages = make([]core.Message, len(cp.messages))
			copy(ctx.Messages, cp.messages)
			cli.turnCount = cp.turn
			// Trim checkpoints after this one
			checkpoints = checkpoints[:idx+1]
			fmt.Printf("  %sZurückgespult zu Turn %d (%d Messages)%s\n",
				colorGreen, cp.turn, len(ctx.Messages), colorReset)
			return
		}
	}
	fmt.Printf("  %sKein Checkpoint bei Turn %d gefunden.%s\n", colorRed, target, colorReset)
}

func (cli *CLIBridge) rewindLast(ctx *core.Context) {
	if len(checkpoints) == 0 {
		fmt.Printf("  %sKeine Checkpoints gespeichert. Nutze /checkpoint%s\n", colorGray, colorReset)
		return
	}
	cp := checkpoints[len(checkpoints)-1]
	checkpoints = checkpoints[:len(checkpoints)-1]
	ctx.Messages = make([]core.Message, len(cp.messages))
	copy(ctx.Messages, cp.messages)
	cli.turnCount = cp.turn
	fmt.Printf("  %sZurückgespult zu Turn %d (%d Messages)%s\n",
		colorGreen, cp.turn, len(ctx.Messages), colorReset)
}

func (cli *CLIBridge) listSessions() {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")

	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) == 0 {
		fmt.Printf("  %sKeine Sessions gefunden.%s\n", colorGray, colorReset)
		return
	}

	fmt.Printf("  %sSessions:%s\n", colorBold, colorReset)
	count := 0
	for idx := len(entries) - 1; idx >= 0 && count < 10; idx-- {
		name := entries[idx].Name()
		if strings.HasSuffix(name, ".json") && !strings.Contains(name, "transcript") {
			info, _ := entries[idx].Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf("(%d KB)", info.Size()/1024)
			}
			fmt.Printf("  %s%-40s%s %s\n", colorCyan, name, colorReset, size)
			count++
		}
	}
	fmt.Printf("  %sNutze /resume <id> zum Laden%s\n", colorGray, colorReset)
}

func (cli *CLIBridge) resumeSessionByID(id string, ctx *core.Context) {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")

	entries, _ := os.ReadDir(logDir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), id) && strings.HasSuffix(entry.Name(), ".json") {
			path := filepath.Join(logDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Printf("  %sFehler: %v%s\n", colorRed, err, colorReset)
				return
			}
			var log core.ChatLog
			if err := json.Unmarshal(data, &log); err != nil {
				fmt.Printf("  %sFehler: %v%s\n", colorRed, err, colorReset)
				return
			}
			ctx.Messages = nil
			for _, entry := range log.Entries {
				if entry.Role == "user" || entry.Role == "assistant" {
					ctx.AddMessage(entry.Role, entry.Content)
				}
			}
			fmt.Printf("  %sSession geladen: %s (%d Messages)%s\n",
				colorGreen, log.SessionID, len(ctx.Messages), colorReset)
			return
		}
	}
	fmt.Printf("  %sSession %q nicht gefunden.%s\n", colorRed, id, colorReset)
}

func (cli *CLIBridge) exportSession(ctx *core.Context) string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".neo-heinzel", "exports")
	os.MkdirAll(dir, 0755)

	name := ctx.SessionID
	if cli.sessionName != "" {
		name = strings.ReplaceAll(cli.sessionName, " ", "-")
	}
	filename := fmt.Sprintf("%s_%s.md", time.Now().Format("2006-01-02"), name)
	path := filepath.Join(dir, filename)

	var lines []string
	lines = append(lines, fmt.Sprintf("# Session: %s", name))
	lines = append(lines, fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02 15:04")))

	for _, msg := range ctx.Messages {
		switch msg.Role {
		case "user":
			lines = append(lines, fmt.Sprintf("## User\n\n%s\n", msg.Content))
		case "assistant":
			lines = append(lines, fmt.Sprintf("## Assistant\n\n%s\n", msg.Content))
		case "tool":
			lines = append(lines, fmt.Sprintf("## Tool\n\n```\n%s\n```\n", msg.Content))
		}
	}

	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	return path
}

func (cli *CLIBridge) resumeSession(ctx *core.Context) {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")

	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) == 0 {
		fmt.Printf("  %sKeine Sessions gefunden.%s\n", colorGray, colorReset)
		return
	}

	var latest string
	for idx := len(entries) - 1; idx >= 0; idx-- {
		name := entries[idx].Name()
		if strings.HasSuffix(name, ".json") && !strings.Contains(name, "transcript") {
			latest = filepath.Join(logDir, name)
			break
		}
	}

	if latest == "" {
		fmt.Printf("  %sKeine Sessions gefunden.%s\n", colorGray, colorReset)
		return
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		fmt.Printf("  %sFehler: %v%s\n", colorRed, err, colorReset)
		return
	}

	var log core.ChatLog
	if err := json.Unmarshal(data, &log); err != nil {
		fmt.Printf("  %sFehler: %v%s\n", colorRed, err, colorReset)
		return
	}

	ctx.Messages = nil
	for _, entry := range log.Entries {
		if entry.Role == "user" || entry.Role == "assistant" {
			ctx.AddMessage(entry.Role, entry.Content)
		}
	}

	fmt.Printf("  %sSession geladen: %s (%d Messages, %s)%s\n",
		colorGreen, log.SessionID, len(ctx.Messages),
		log.StartTime.Format("2006-01-02 15:04"), colorReset)
}

func (cli *CLIBridge) handleSpeakCmd(parts []string, ctx *core.Context, loop *core.Loop) bool {
	text := cli.lastOutput
	if len(parts) > 1 {
		text = strings.Join(parts[1:], " ")
	}
	if text == "" {
		fmt.Println("Nothing to speak.")
		return false
	}

	providerURL := ""
	for _, name := range loop.Dispatcher.ListAddons() {
		addon, ok := loop.Dispatcher.GetAddon(name)
		if !ok {
			continue
		}
		if hp, ok := addon.(*HTTPProvider); ok {
			providerURL = hp.BaseURL()
			break
		}
	}
	if providerURL == "" {
		fmt.Println("No provider URL available.")
		return false
	}

	reqBody, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(providerURL+"/speak", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		fmt.Printf("%sSpeak error: %v%s\n", colorRed, err, colorReset)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%sSpeak error: %s%s\n", colorRed, string(body), colorReset)
		return false
	}

	tmpFile, err := os.CreateTemp("", "heinzel-speak-*.mp3")
	if err != nil {
		fmt.Printf("%sCan't create temp file: %v%s\n", colorRed, err, colorReset)
		return false
	}
	defer os.Remove(tmpFile.Name())

	io.Copy(tmpFile, resp.Body)
	tmpFile.Close()

	for _, player := range []string{"mpv", "ffplay", "aplay", "paplay"} {
		cmd := exec.Command(player, "--no-video", tmpFile.Name())
		if player != "mpv" {
			cmd = exec.Command(player, tmpFile.Name())
		}
		cmd.Stderr = nil
		cmd.Stdout = nil
		if err := cmd.Run(); err == nil {
			return false
		}
	}
	fmt.Printf("%sNo audio player found (tried mpv, ffplay, aplay, paplay)%s\n", colorYellow, colorReset)
	return false
}
