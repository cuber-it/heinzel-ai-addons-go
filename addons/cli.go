// CLIBridge — interactive terminal IO for the agent.

package addons

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// ANSI colors
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
	colorBold    = "\033[1m"
)

const maxInputBytes = 1024 * 1024 // 1 MB scanner buffer

func colorize(color, text string) string { return color + text + colorReset }

// cliHandler processes a slash command. parts is the split command line.
// Returns true if the session should exit.
type cliHandler func(parts []string, ctx *core.Context, loop *core.Loop) bool

type CLIBridge struct {
	core.BaseAddon
	prompt          string
	showThinking    bool
	history         []string
	sessionStart    time.Time
	turnCount       int
	historyFile     string
	rcFiles         []string
	sessionName     string
	lastInput       string
	lastOutput      string
	startupDocs     []core.StartupDoc
	commandHandlers map[string]cliHandler
}

func NewCLIBridge(prompt string, showThinking bool) *CLIBridge {
	home, _ := os.UserHomeDir()
	bridge := &CLIBridge{
		prompt:       prompt,
		showThinking: showThinking,
		historyFile:  filepath.Join(home, ".neo-heinzel", "history"),
	}
	bridge.initCommandHandlers()
	return bridge
}

func (cli *CLIBridge) Name() string               { return "cli" }
func (cli *CLIBridge) Type() core.AddonType     { return core.AddonTool }
func (cli *CLIBridge) Start() error                { return nil }
func (cli *CLIBridge) Stop() error                 { return nil }

func (cli *CLIBridge) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnSessionStart,
		core.OnSessionEnd,
	}
}

func (cli *CLIBridge) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnSessionStart:
		cli.sessionStart = time.Now()
		fmt.Printf("%sNeo-Heinzel ready.%s\n", colorGreen, colorReset)
	case core.OnSessionEnd:
		duration := time.Since(cli.sessionStart).Round(time.Second)
		fmt.Printf("%sBye. (%d turns, %s)%s\n", colorGreen, cli.turnCount, duration, colorReset)
	}
	return core.Result{}
}

func (cli *CLIBridge) Drive(loop *core.Loop) {
	ctx := cli.setupSession(loop)
	defer func() {
		loop.Dispatcher.Dispatch(core.OnSessionEnd, ctx)
		cli.saveHistory()
	}()

	cancelCh := make(chan os.Signal, 1)
	signal.Notify(cancelCh, os.Interrupt)
	go func() {
		for range cancelCh {
			ctx.Halt = true
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0), maxInputBytes)

	cli.printPrompt(ctx)
	for scanner.Scan() {
		if cli.processInput(scanner, loop, ctx) {
			break
		}
		cli.printPrompt(ctx)
	}
}

func (cli *CLIBridge) setupSession(loop *core.Loop) *core.Context {
	ctx := core.NewContext("cli")

	core.InitThinking(ctx, func(step core.ThinkingStep) {
		if cli.showThinking {
			cli.printThinkingStep(step)
		}
	})

	ctx.OnToken = func(token string) {
		fmt.Print(token)
	}

	cli.loadHistory()
	loop.Dispatcher.Dispatch(core.OnSessionStart, ctx)
	cli.detectProject(ctx)
	cli.loadStartupDocs(cli.startupDocs, ctx)
	cli.loadRC(loop, ctx)

	return ctx
}

func (cli *CLIBridge) processInput(scanner *bufio.Scanner, loop *core.Loop, ctx *core.Context) bool {
	trimmed := strings.TrimSpace(scanner.Text())

	if trimmed == "" {
		return false
	}

	if strings.HasPrefix(trimmed, "/") {
		return cli.handleCommand(trimmed, loop, ctx)
	}

	if strings.HasPrefix(trimmed, "!") {
		bashCmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
		if bashCmd != "" {
			cli.executeBash(bashCmd, ctx)
		}
		return false
	}

	for strings.HasSuffix(trimmed, "\\") {
		trimmed = strings.TrimSuffix(trimmed, "\\")
		fmt.Print("  ... ")
		if scanner.Scan() {
			trimmed += "\n" + scanner.Text()
		}
	}

	cli.history = append(cli.history, trimmed)
	cli.lastInput = trimmed
	cli.turnCount++

	ctx.Set(KeyOutputStreamed, nil)
	output := loop.Run(ctx, trimmed)
	cli.lastOutput = output

	if ctx.Error != nil {
		fmt.Printf("%sError: %v%s\n", colorRed, ctx.Error, colorReset)
	}

	if output != "" {
		if _, ok := ctx.Get(KeyOutputStreamed); !ok {
			fmt.Printf("%s%s%s\n", colorBold, output, colorReset)
		}
	}

	return false
}

func (cli *CLIBridge) printPrompt(ctx *core.Context) {
	strategyStr := ""
	if val, ok := ctx.Get(KeyStrategy); ok {
		if strategy, ok := val.(Strategy); ok && strategy != StrategyPassthrough {
			strategyStr = fmt.Sprintf("%s[%s]%s ", colorCyan, strategy, colorReset)
		}
	}
	fmt.Printf("%s%s%s", strategyStr, colorGreen, cli.prompt)
	fmt.Print(colorReset)
}

func (cli *CLIBridge) printThinkingStep(step core.ThinkingStep) {
	color := colorGray
	prefix := "  "
	switch step.Type {
	case "classify":
		color = colorCyan
	case "think":
		color = colorYellow
	case "memory":
		color = colorMagenta
	case "validate":
		color = colorBlue
	case "backtrack":
		color = colorRed
		prefix = "  << "
	case "checkpoint":
		color = colorGray
	case "tool":
		color = colorGreen
	}
	fmt.Printf("%s%s%s%s\n", prefix, color, step, colorReset)
}
