// Heinzel Addons Testrunner — interactive agent with dynamic addon loading.
// Tests the full stack: Core + Provider + Addons.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
	"github.com/cuber-it/heinzel-ai-addons-go/addons"
)

// Available addons and their constructors
type addonFactory struct {
	name        string
	description string
	create      func(dispatcher *core.Dispatcher) core.Addon
	priority    int
}

var availableAddons []addonFactory

func initAddonRegistry(providerURL, model string) {
	availableAddons = []addonFactory{
		// Provider
		{"http-provider", "LLM Provider (HTTP)", func(d *core.Dispatcher) core.Addon {
			return addons.NewHTTPProvider("openai", providerURL, model)
		}, 100},
		{"echo", "Echo Provider (test)", func(d *core.Dispatcher) core.Addon {
			return &addons.EchoProvider{}
		}, 100},

		// Control
		{"commands", "Universal slash-commands", func(d *core.Dispatcher) core.Addon {
			return addons.NewCommandAddon(d)
		}, 1},
		{"costguard", "Token budget enforcement", func(d *core.Dispatcher) core.Addon {
			return addons.NewCostGuardAddon()
		}, 2},
		{"recovery", "Error handling + circuit breaker", func(d *core.Dispatcher) core.Addon {
			return addons.NewRecoveryAddon()
		}, 0},
		{"prompt", "Prompt composition + awareness", func(d *core.Dispatcher) core.Addon {
			return addons.NewPromptAddon("Du bist Heinzel, ein kognitiver Agent.", d, nil)
		}, 5},

		// Reasoning
		{"reasoning", "Strategy + triage + multi-step thinking", func(d *core.Dispatcher) core.Addon {
			addon := addons.NewReasoningAddon(3)
			addon.SetDispatcher(d)
			return addon
		}, 10},

		// Memory
		{"memory-composer", "Memory layer orchestration", func(d *core.Dispatcher) core.Addon {
			mc := addons.NewMemoryComposerAddon(16000)
			mc.Composer().Register(addons.NewFactsLayer())
			mc.Composer().Register(addons.NewRecentMessagesLayer(50))
			mc.Composer().Register(addons.NewSessionSummaryLayer(2000))
			mc.Composer().Register(&addons.ToolsLayer{})
			return mc
		}, 15},
		{"compaction", "Context compaction", func(d *core.Dispatcher) core.Addon {
			return addons.NewCompactionAddon(addons.NewFactsLayer(), addons.NewSessionSummaryLayer(2000))
		}, 16},

		// Tools
		{"mcp-manager", "MCP server management", func(d *core.Dispatcher) core.Addon {
			home, _ := os.UserHomeDir()
			return addons.NewMCPManager(d, home+"/.heinzel/permissions.yaml")
		}, 20},
		{"fileupload", "File injection (@path)", func(d *core.Dispatcher) core.Addon {
			return addons.NewFileUploadAddon(10)
		}, 7},
		{"websearch", "Web search + URL fetch", func(d *core.Dispatcher) core.Addon {
			return addons.NewWebSearchAddon()
		}, 50},

		// Logging
		{"chatlog", "Session log to disk", func(d *core.Dispatcher) core.Addon {
			home, _ := os.UserHomeDir()
			return addons.NewChatLogAddon(home + "/.heinzel/logs")
		}, 90},
		{"transcript", "Numbered turn protocol", func(d *core.Dispatcher) core.Addon {
			home, _ := os.UserHomeDir()
			return addons.NewTranscriptAddon(home + "/.heinzel/logs")
		}, 91},
		{"logger", "Debug hook observer", func(d *core.Dispatcher) core.Addon {
			return addons.NewLogger(true)
		}, 1},
	}
}

func main() {
	providerURL := flag.String("provider", "", "LLM provider URL (empty = echo)")
	model := flag.String("model", "gpt-4.1", "model name")
	loadAddons := flag.String("addons", "", "comma-separated addon names to load at start")
	loadAll := flag.Bool("all", false, "load all addons")
	flag.Parse()

	dispatcher := core.NewDispatcher()
	initAddonRegistry(*providerURL, *model)

	// Load provider
	if *providerURL == "" {
		loadAddon(dispatcher, "echo")
		fmt.Println("Heinzel Addons Testrunner (echo mode)")
		fmt.Printf("  Use --provider http://thebrain:12101 for real LLM\n")
	} else {
		loadAddon(dispatcher, "http-provider")
		fmt.Printf("Heinzel Addons Testrunner (provider: %s, model: %s)\n", *providerURL, *model)
	}

	// Load requested addons
	if *loadAll {
		for _, af := range availableAddons {
			if af.name != "echo" && af.name != "http-provider" {
				loadAddon(dispatcher, af.name)
			}
		}
	} else if *loadAddons != "" {
		for _, name := range strings.Split(*loadAddons, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				loadAddon(dispatcher, name)
			}
		}
	}

	if err := dispatcher.StartAll(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer dispatcher.StopAll()

	loop := core.NewLoop(dispatcher)
	ctx := core.NewContext("addons-test")
	ctx.Prompts = core.NewPromptManager()
	ctx.Prompts.Set(core.LayerSystem, "core", "Du bist Heinzel, ein kognitiver Agent. Antworte präzise und hilfreich.", 0)
	ctx.SystemPrompt = ctx.Prompts.Compose()

	// Wire addons that need loop access
	wireLoopDependencies(dispatcher, loop)

	dispatcher.Dispatch(core.OnSessionStart, ctx)

	fmt.Printf("Session: %s | Addons: %d | Hooks: %d\n",
		ctx.SessionID, len(dispatcher.ListAddons()), int(core.HookCount))
	fmt.Println("Type /help for commands, /quit to exit.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			fmt.Print("> ")
			continue
		}

		if strings.HasPrefix(input, "/") {
			if handleCommand(input, dispatcher, loop, ctx) {
				break
			}
			fmt.Print("> ")
			continue
		}

		// Set up token streaming
		ctx.OnToken = func(token string) { fmt.Print(token) }
		output := loop.Run(ctx, input)
		if output != "" {
			if _, ok := ctx.Get(addons.KeyOutputStreamed); !ok {
				fmt.Println(output)
			}
		}
		fmt.Println()
		fmt.Print("> ")
	}
}

func loadAddon(dispatcher *core.Dispatcher, name string) {
	for _, af := range availableAddons {
		if af.name == name {
			addon := af.create(dispatcher)
			if err := dispatcher.Register(addon, af.priority); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				fmt.Printf("  Loaded: %s (%s)\n", af.name, af.description)
			}
			return
		}
	}
	fmt.Printf("  Unknown addon: %s\n", name)
}

func wireLoopDependencies(dispatcher *core.Dispatcher, loop *core.Loop) {
	for _, name := range dispatcher.ListAddons() {
		addon, ok := dispatcher.GetAddon(name)
		if !ok {
			continue
		}
		if r, ok := addon.(*addons.ReasoningAddon); ok {
			r.SetLoop(loop)
		}
		if c, ok := addon.(*addons.CompactionAddon); ok {
			c.SetLoop(loop)
		}
	}
}

func handleCommand(cmd string, dispatcher *core.Dispatcher, loop *core.Loop, ctx *core.Context) bool {
	parts := strings.Fields(cmd)
	command := strings.ToLower(parts[0])

	switch command {
	case "/quit", "/exit", "/q":
		fmt.Println("Bye.")
		return true

	case "/help":
		fmt.Println("Testrunner commands:")
		fmt.Println("  /addon list             Available addons")
		fmt.Println("  /addon load <name>      Load an addon")
		fmt.Println("  /addon unload <name>    Unload an addon")
		fmt.Println("  /addons                 Show loaded addons")
		fmt.Println("  /hooks                  Show hook points")
		fmt.Println("  /keys                   Show context keys")
		fmt.Println("  /status                 Session info")
		fmt.Println("  /context [system <t>]   Show/set prompt")
		fmt.Println("  /messages [clear]       Show/clear messages")
		fmt.Println("  /quit                   Exit")
		fmt.Println()
		fmt.Println("Addon commands (if loaded):")
		for _, cmd := range dispatcher.AllCommands() {
			fmt.Printf("  /%-20s %s\n", cmd.Name, cmd.Description)
		}

	case "/addon":
		if len(parts) < 2 {
			fmt.Println("  Usage: /addon [list|load <name>|unload <name>]")
			break
		}
		switch parts[1] {
		case "list":
			fmt.Println("  Available addons:")
			loaded := dispatcher.ListAddons()
			for _, af := range availableAddons {
				status := " "
				for _, l := range loaded {
					if l == af.name {
						status = "*"
						break
					}
				}
				fmt.Printf("  %s %-20s %s\n", status, af.name, af.description)
			}
		case "load":
			if len(parts) < 3 {
				fmt.Println("  Usage: /addon load <name>")
				break
			}
			loadAddon(dispatcher, parts[2])
			dispatcher.StartAll()
			wireLoopDependencies(dispatcher, loop)
		case "unload":
			if len(parts) < 3 {
				fmt.Println("  Usage: /addon unload <name>")
				break
			}
			dispatcher.Unregister(parts[2])
			fmt.Printf("  Unloaded: %s\n", parts[2])
		default:
			fmt.Println("  Usage: /addon [list|load <name>|unload <name>]")
		}

	case "/addons":
		loaded := dispatcher.ListAddons()
		if len(loaded) == 0 {
			fmt.Println("  (none)")
		}
		for _, name := range loaded {
			addon, ok := dispatcher.GetAddon(name)
			if ok {
				fmt.Printf("  %-20s [%s]\n", name, addon.Type())
			}
		}

	case "/hooks":
		for hook := core.HookPoint(0); hook < core.HookCount; hook++ {
			subs := dispatcher.HookSubscribers(hook)
			if len(subs) > 0 {
				fmt.Printf("  %-25s %s\n", hook, strings.Join(subs, ", "))
			}
		}

	case "/keys":
		keys := core.AllKeys()
		for _, key := range keys {
			val, ok := ctx.Get(key.Name)
			valStr := "(not set)"
			if ok {
				valStr = fmt.Sprintf("%v", val)
			}
			fmt.Printf("  %-30s %-20s %s\n", key.Name, valStr, key.Description)
		}

	case "/status":
		fmt.Printf("  Session:  %s\n", ctx.SessionID)
		fmt.Printf("  Messages: %d\n", len(ctx.Messages))
		fmt.Printf("  Tokens:   ~%d\n", ctx.TokenEstimate())
		fmt.Printf("  Addons:   %d\n", len(dispatcher.ListAddons()))
		fmt.Printf("  Halt:     %v\n", ctx.Halt)
		fmt.Printf("  Error:    %v\n", ctx.Error)

	case "/context":
		if len(parts) >= 3 && parts[1] == "system" {
			text := strings.Join(parts[2:], " ")
			ctx.Prompts.Set(core.LayerSystem, "user", text, 0)
			ctx.SystemPrompt = ctx.Prompts.Compose()
			fmt.Printf("  System prompt set.\n")
		} else {
			composed := ctx.Prompts.Compose()
			if composed == "" {
				fmt.Println("  (empty)")
			} else {
				fmt.Println(composed)
			}
		}

	case "/messages":
		if len(parts) >= 2 && parts[1] == "clear" {
			ctx.Messages = nil
			fmt.Println("  Cleared.")
		} else {
			for idx, msg := range ctx.Messages {
				preview := msg.Content
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				fmt.Printf("  %3d [%s] %s\n", idx+1, msg.Role, preview)
			}
		}

	default:
		// Try addon commands
		cmdName := strings.TrimPrefix(command, "/")
		result, handled := dispatcher.DispatchCommand(cmdName, strings.Join(parts[1:], " "), ctx)
		if handled {
			if result != "" {
				fmt.Println(result)
			}
		} else {
			fmt.Printf("  Unknown: %s (try /help)\n", command)
		}
	}

	return false
}
