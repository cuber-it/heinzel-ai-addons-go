// Command handlers for status and diff display.

package addons

import (
	"fmt"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func (addon *CommandAddon) showStatus(ctx *core.Context) string {
	name := ctx.SessionID
	if val, ok := ctx.Get(KeySessionName); ok {
		if s, ok := val.(string); ok && s != "" {
			name = s
		}
	}
	return fmt.Sprintf("Session:  %s\nTurns:    %d\nMessages: %d\nAddons:   %d",
		name, addon.turnCount, len(ctx.Messages), len(addon.dispatcher.ListAddons()))
}

func (addon *CommandAddon) showDiff(ctx *core.Context) string {
	var outputs []string
	for _, msg := range ctx.Messages {
		if msg.Role == "assistant" {
			outputs = append(outputs, msg.Content)
		}
	}
	if len(outputs) < 2 {
		return "Nicht genug Turns für einen Diff."
	}
	prev := strings.Split(outputs[len(outputs)-2], "\n")
	curr := strings.Split(outputs[len(outputs)-1], "\n")

	var lines []string
	maxLines := len(prev)
	if len(curr) > maxLines {
		maxLines = len(curr)
	}
	for idx := 0; idx < maxLines; idx++ {
		prevLine, currLine := "", ""
		if idx < len(prev) {
			prevLine = prev[idx]
		}
		if idx < len(curr) {
			currLine = curr[idx]
		}
		if prevLine != currLine {
			if prevLine != "" {
				lines = append(lines, "- "+prevLine)
			}
			if currLine != "" {
				lines = append(lines, "+ "+currLine)
			}
		}
	}
	if len(lines) == 0 {
		return "Kein Unterschied."
	}
	return strings.Join(lines, "\n")
}
