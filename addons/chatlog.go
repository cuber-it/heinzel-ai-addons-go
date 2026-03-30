// ChatLogAddon — writes conversation log to disk on session end.

package addons

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

type ChatLogAddon struct {
	core.BaseAddon
	logDir string
}

func NewChatLogAddon(logDir string) *ChatLogAddon {
	return &ChatLogAddon{logDir: logDir}
}

func (addon *ChatLogAddon) Name() string           { return "chatlog" }
func (addon *ChatLogAddon) Type() core.AddonType { return core.AddonObserver }
func (addon *ChatLogAddon) Start() error {
	if addon.logDir != "" {
		return os.MkdirAll(addon.logDir, 0755)
	}
	return nil
}
func (addon *ChatLogAddon) Stop() error { return nil }

func (addon *ChatLogAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnSessionEnd,
	}
}

func (addon *ChatLogAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "log", Description: "conversation log", Usage: "/log [stats|save|show]"},
	}
}

func (addon *ChatLogAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	switch args {
	case "", "stats":
		tokIn, tokOut := ctx.Log.TokensTotal()
		return fmt.Sprintf("Log entries: %d\nTokens in: %d\nTokens out: %d\nSession: %s\nDuration: %s",
			ctx.Log.Count(), tokIn, tokOut,
			ctx.Log.SessionID,
			time.Since(ctx.Log.StartTime).Round(time.Second))
	case "save":
		path := addon.save(ctx)
		if path != "" {
			return fmt.Sprintf("Saved to: %s", path)
		}
		return "Error saving log."
	case "show":
		data, err := json.MarshalIndent(ctx.Log, "", "  ")
		if err != nil {
			return fmt.Sprintf("Error marshaling log: %v", err)
		}
		return string(data)
	default:
		return "Usage: /log [stats|save|show]"
	}
}

func (addon *ChatLogAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook == core.OnSessionEnd {
		addon.save(ctx)
	}
	return core.Result{}
}

func (addon *ChatLogAddon) save(ctx *core.Context) string {
	if addon.logDir == "" || ctx.Log.Count() == 0 {
		return ""
	}

	filename := fmt.Sprintf("%s_%s.json",
		time.Now().Format("2006-01-02_150405"),
		ctx.Log.SessionID)
	path := filepath.Join(addon.logDir, filename)

	data, err := json.MarshalIndent(ctx.Log, "", "  ")
	if err != nil {
		return ""
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}

	return path
}
