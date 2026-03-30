// Logger — debug observer that logs all hook dispatches.

package addons

import (
	"fmt"
	"os"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

type Logger struct {
	core.BaseAddon
	verbose bool
}

func NewLogger(verbose bool) *Logger {
	return &Logger{verbose: verbose}
}

func (logger *Logger) Name() string               { return "logger" }
func (logger *Logger) Type() core.AddonType     { return core.AddonObserver }
func (logger *Logger) Start() error                { return nil }
func (logger *Logger) Stop() error                 { return nil }

func (logger *Logger) Hooks() []core.HookPoint {
	if logger.verbose {
		hooks := make([]core.HookPoint, core.HookCount)
		for idx := range hooks {
			hooks[idx] = core.HookPoint(idx)
		}
		return hooks
	}
	return []core.HookPoint{
		core.OnInput, core.OnOutput,
		core.OnLLMError, core.OnToolError, core.OnLoopAbort,
	}
}

func (logger *Logger) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	timestamp := time.Now().Format("15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, hook)
	return core.Result{}
}
