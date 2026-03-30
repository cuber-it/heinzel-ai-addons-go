// Session management commands — checkpoint, rewind, resume, export, import.

package addons

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// Checkpoint system
type cmdCheckpoint struct {
	turn     int
	messages []core.Message
}

var cmdCheckpoints []cmdCheckpoint

func (addon *CommandAddon) saveCheckpoint(ctx *core.Context) {
	msgs := make([]core.Message, len(ctx.Messages))
	copy(msgs, ctx.Messages)
	cmdCheckpoints = append(cmdCheckpoints, cmdCheckpoint{
		turn:     addon.turnCount,
		messages: msgs,
	})
}

func (addon *CommandAddon) rewindTo(target int, ctx *core.Context) string {
	for idx := len(cmdCheckpoints) - 1; idx >= 0; idx-- {
		if cmdCheckpoints[idx].turn <= target {
			cp := cmdCheckpoints[idx]
			ctx.Messages = make([]core.Message, len(cp.messages))
			copy(ctx.Messages, cp.messages)
			addon.turnCount = cp.turn
			cmdCheckpoints = cmdCheckpoints[:idx+1]
			return fmt.Sprintf("Zurückgespult zu Turn %d (%d Messages)", cp.turn, len(ctx.Messages))
		}
	}
	return fmt.Sprintf("Kein Checkpoint bei Turn %d", target)
}

func (addon *CommandAddon) rewindLast(ctx *core.Context) string {
	if len(cmdCheckpoints) == 0 {
		return "Keine Checkpoints. Nutze /checkpoint"
	}
	cp := cmdCheckpoints[len(cmdCheckpoints)-1]
	cmdCheckpoints = cmdCheckpoints[:len(cmdCheckpoints)-1]
	ctx.Messages = make([]core.Message, len(cp.messages))
	copy(ctx.Messages, cp.messages)
	addon.turnCount = cp.turn
	return fmt.Sprintf("Zurückgespult zu Turn %d (%d Messages)", cp.turn, len(ctx.Messages))
}

func (addon *CommandAddon) listSessions() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) == 0 {
		return "Keine Sessions gefunden."
	}
	var lines []string
	count := 0
	for idx := len(entries) - 1; idx >= 0 && count < 10; idx-- {
		name := entries[idx].Name()
		if strings.HasSuffix(name, ".json") && !strings.Contains(name, "transcript") {
			lines = append(lines, "  "+name)
			count++
		}
	}
	return "Sessions:\n" + strings.Join(lines, "\n") + "\n\nNutze /resume <id>"
}

func (addon *CommandAddon) resumeLatest(ctx *core.Context) string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return "Keine Sessions gefunden."
	}
	for idx := len(entries) - 1; idx >= 0; idx-- {
		name := entries[idx].Name()
		if strings.HasSuffix(name, ".json") && !strings.Contains(name, "transcript") {
			return addon.loadSession(filepath.Join(logDir, name), ctx)
		}
	}
	return "Keine Sessions gefunden."
}

func (addon *CommandAddon) resumeByID(id string, ctx *core.Context) string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".neo-heinzel", "logs")
	entries, _ := os.ReadDir(logDir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), id) && strings.HasSuffix(entry.Name(), ".json") {
			return addon.loadSession(filepath.Join(logDir, entry.Name()), ctx)
		}
	}
	return fmt.Sprintf("Session %q nicht gefunden.", id)
}

func (addon *CommandAddon) loadSession(path string, ctx *core.Context) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Fehler: %v", err)
	}
	var log core.ChatLog
	if err := json.Unmarshal(data, &log); err != nil {
		return fmt.Sprintf("Fehler: %v", err)
	}
	ctx.Messages = nil
	for _, entry := range log.Entries {
		if entry.Role == "user" || entry.Role == "assistant" {
			ctx.AddMessage(entry.Role, entry.Content)
		}
	}
	return fmt.Sprintf("Session geladen: %s (%d Messages)", log.SessionID, len(ctx.Messages))
}

func (addon *CommandAddon) exportSession(ctx *core.Context) string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".neo-heinzel", "exports")
	os.MkdirAll(dir, 0755)

	name := ctx.SessionID
	if val, ok := ctx.Get(KeySessionName); ok {
		if s, ok := val.(string); ok && s != "" {
			name = strings.ReplaceAll(s, " ", "-")
		}
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%s.md", time.Now().Format("2006-01-02"), name))

	var lines []string
	lines = append(lines, fmt.Sprintf("# Session: %s\n", name))
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
	return "Exportiert: " + path
}

func (addon *CommandAddon) importFiles(pattern string, ctx *core.Context) string {
	if strings.HasPrefix(pattern, "http://") || strings.HasPrefix(pattern, "https://") {
		ctx.Prompts.Add(core.LayerTurn, "import:"+pattern, "URL: "+pattern, 70)
		return "URL in Kontext: " + pattern
	}

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		if _, err := os.Stat(pattern); err == nil {
			matches = []string{pattern}
		} else {
			return "Keine Dateien gefunden: " + pattern
		}
	}

	total := 0
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() > 10*1024*1024 {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		ctx.Prompts.Add(core.LayerTurn, "import:"+name,
			fmt.Sprintf("[File: %s]\n```\n%s\n```", name, string(content)), 60)
		total++
	}
	return fmt.Sprintf("%d Datei(en) in Kontext geladen", total)
}
