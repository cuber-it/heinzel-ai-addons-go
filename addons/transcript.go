// TranscriptAddon — numbered turn protocol with search and context injection.

package addons

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const previewLength = 60

// TranscriptEntry is one complete turn in the full protocol
type TranscriptEntry struct {
	Number    int                    `json:"number"`
	Time      time.Time              `json:"time"`
	Input     string                 `json:"input"`
	Output    string                 `json:"output"`
	Strategy  string                 `json:"strategy,omitempty"`
	Thinking  []string               `json:"thinking,omitempty"`
	ToolCalls []TranscriptToolCall   `json:"tool_calls,omitempty"`
	Tokens    int                    `json:"tokens,omitempty"`
	Duration  string                 `json:"duration,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type TranscriptToolCall struct {
	Name   string `json:"name"`
	Args   string `json:"args,omitempty"`
	Result string `json:"result,omitempty"`
}

// TranscriptAddon writes every turn as numbered, complete JSON record
// This is the "Rückgedächtnis" — full protocol, no compression loss
type TranscriptAddon struct {
	core.BaseAddon
	dir       string
	file      *os.File
	mu        sync.Mutex
	counter   int
	sessionID string
	turnStart time.Time
}

func NewTranscriptAddon(dir string) *TranscriptAddon {
	return &TranscriptAddon{dir: dir}
}

func (addon *TranscriptAddon) Name() string           { return "transcript" }
func (addon *TranscriptAddon) Type() core.AddonType { return core.AddonObserver }

func (addon *TranscriptAddon) Start() error {
	if addon.dir != "" {
		os.MkdirAll(addon.dir, 0755)
	}
	return nil
}

func (addon *TranscriptAddon) Stop() error {
	if addon.file != nil {
		addon.file.Close()
	}
	return nil
}

func (addon *TranscriptAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnSessionStart,
		core.OnInput,
		core.OnOutput,
		core.OnSessionEnd,
	}
}

func (addon *TranscriptAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "recall", Description: "recall a turn by number from the transcript",
			Usage: "/recall <number> — show full turn\n  /recall last [N] — show last N turns\n  /recall search <text> — find turns containing text"},
	}
}

func (addon *TranscriptAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return fmt.Sprintf("Transcript: %d turns recorded.\nFile: %s", addon.counter, addon.filePath())
	}

	switch parts[0] {
	case "last":
		limit := 5
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &limit)
		}
		return addon.showLast(limit)

	case "search":
		if len(parts) < 2 {
			return "Usage: /recall search <text>"
		}
		query := strings.Join(parts[1:], " ")
		return addon.searchTranscript(query)

	case "inject":
		// Inject without showing — silent recall into context
		if len(parts) < 2 {
			return "Usage: /recall inject <number>"
		}
		var num int
		fmt.Sscanf(parts[1], "%d", &num)
		if addon.InjectRecall(num, ctx) {
			return fmt.Sprintf("Turn #%d in Kontext injiziert.", num)
		}
		return fmt.Sprintf("Turn #%d nicht gefunden.", num)

	default:
		// Try as number — show AND inject
		var num int
		if _, err := fmt.Sscanf(parts[0], "%d", &num); err == nil {
			addon.InjectRecall(num, ctx) // inject into context
			return addon.recallTurn(num)  // show to user
		}
		return "Usage: /recall <number> | /recall last [N] | /recall search <text> | /recall inject <N>"
	}
}

func (addon *TranscriptAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnSessionStart:
		addon.sessionID = ctx.SessionID
		addon.openFile()

	case core.OnInput:
		addon.turnStart = time.Now()

	case core.OnOutput:
		addon.mu.Lock()
		defer addon.mu.Unlock()

		addon.counter++
		entry := TranscriptEntry{
			Number:   addon.counter,
			Time:     time.Now(),
			Input:    ctx.Input,
			Output:   ctx.Output,
			Duration: time.Since(addon.turnStart).Round(time.Millisecond).String(),
		}

	if val, ok := ctx.Get(KeyStrategy); ok {
			entry.Strategy = fmt.Sprintf("%v", val)
		}

	if thinking := core.GetThinking(ctx); thinking != nil {
			for _, step := range thinking.Steps {
				entry.Thinking = append(entry.Thinking, step.String())
			}
		}

	for _, tc := range ctx.ToolCalls {
			entry.ToolCalls = append(entry.ToolCalls, TranscriptToolCall{
				Name:   tc.Name,
				Result: tc.Result,
			})
		}

	addon.writeEntry(&entry)
		ctx.Set(KeyLastTurnNumber, addon.counter)

	case core.OnSessionEnd:
		if addon.file != nil {
			addon.file.Close()
			addon.file = nil
		}
	}
	return core.Result{}
}

func (addon *TranscriptAddon) filePath() string {
	if addon.dir == "" {
		return ""
	}
	return filepath.Join(addon.dir, fmt.Sprintf("transcript_%s.jsonl", addon.sessionID))
}

func (addon *TranscriptAddon) openFile() {
	path := addon.filePath()
	if path == "" {
		return
	}
	var err error
	addon.file, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		addon.file = nil
	}
}

func (addon *TranscriptAddon) writeEntry(entry *TranscriptEntry) {
	if addon.file == nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	addon.file.Write(append(data, '\n'))
	addon.file.Sync()
}

// recallTurn loads a specific turn from the transcript file
func (addon *TranscriptAddon) recallTurn(number int) string {
	entries := addon.loadEntries()
	for _, entry := range entries {
		if entry.Number == number {
			return addon.formatEntry(&entry)
		}
	}
	return fmt.Sprintf("Turn #%d not found.", number)
}

// showLast shows the last N turns
func (addon *TranscriptAddon) showLast(limit int) string {
	entries := addon.loadEntries()
	start := 0
	if len(entries) > limit {
		start = len(entries) - limit
	}

	var lines []string
	for _, entry := range entries[start:] {
		preview := entry.Input
		if len(preview) > previewLength {
			preview = preview[:previewLength] + "..."
		}
		lines = append(lines, fmt.Sprintf("  #%-4d %s  %s", entry.Number, entry.Time.Format("15:04:05"), preview))
	}
	if len(lines) == 0 {
		return "No turns recorded."
	}
	return strings.Join(lines, "\n")
}

// searchTranscript finds turns containing text
func (addon *TranscriptAddon) searchTranscript(query string) string {
	entries := addon.loadEntries()
	query = strings.ToLower(query)

	var lines []string
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Input), query) ||
			strings.Contains(strings.ToLower(entry.Output), query) {
			preview := entry.Input
			if len(preview) > previewLength {
				preview = preview[:previewLength] + "..."
			}
			lines = append(lines, fmt.Sprintf("  #%-4d %s", entry.Number, preview))
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("Keine Turns mit %q gefunden.", query)
	}
	return fmt.Sprintf("Gefunden (%d):\n%s", len(lines), strings.Join(lines, "\n"))
}

func (addon *TranscriptAddon) formatEntry(entry *TranscriptEntry) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("=== Turn #%d (%s, %s) ===", entry.Number, entry.Time.Format("2006-01-02 15:04:05"), entry.Duration))
	if entry.Strategy != "" {
		lines = append(lines, fmt.Sprintf("Strategy: %s", entry.Strategy))
	}
	lines = append(lines, fmt.Sprintf("Input:  %s", entry.Input))
	if len(entry.Thinking) > 0 {
		lines = append(lines, "Thinking:")
		for _, step := range entry.Thinking {
			lines = append(lines, "  "+step)
		}
	}
	if len(entry.ToolCalls) > 0 {
		lines = append(lines, "Tools:")
		for _, tc := range entry.ToolCalls {
			lines = append(lines, fmt.Sprintf("  %s → %s", tc.Name, tc.Result))
		}
	}
	lines = append(lines, fmt.Sprintf("Output: %s", entry.Output))
	return strings.Join(lines, "\n")
}

func (addon *TranscriptAddon) loadEntries() []TranscriptEntry {
	path := addon.filePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []TranscriptEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry TranscriptEntry
		if json.Unmarshal([]byte(line), &entry) == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (addon *TranscriptAddon) InjectRecall(number int, ctx *core.Context) bool {
	entries := addon.loadEntries()
	for _, entry := range entries {
		if entry.Number == number {
			content := fmt.Sprintf("Recalled turn #%d:\nUser: %s\nAssistant: %s", entry.Number, entry.Input, entry.Output)
			ctx.Prompts.Add(core.LayerTurn, fmt.Sprintf("recall:%d", number), content, 85)
			return true
		}
	}
	return false
}
