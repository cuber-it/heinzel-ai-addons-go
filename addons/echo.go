// EchoProvider — test provider that echoes input back.

package addons

import (
	"fmt"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

type EchoProvider struct {
	core.BaseAddon
}

func (echo *EchoProvider) Name() string               { return "echo-provider" }
func (echo *EchoProvider) Type() core.AddonType     { return core.AddonProvider }
func (echo *EchoProvider) Start() error                { return nil }
func (echo *EchoProvider) Stop() error                 { return nil }

func (echo *EchoProvider) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnLLMCall}
}

func (echo *EchoProvider) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	ctx.Output = fmt.Sprintf("[echo] %s", ctx.Input)
	return core.Result{}
}
