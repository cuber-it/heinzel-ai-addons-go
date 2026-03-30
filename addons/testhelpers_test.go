package addons

import (
	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// testContext creates a minimal context with thinking initialized.
// Use this as the starting point for all addon tests.
func testContext() *core.Context {
	ctx := core.NewContext("test")
	core.InitThinking(ctx, nil)
	return ctx
}

// testDispatcher creates a dispatcher with the given addons registered at default priority.
func testDispatcher(addons ...core.Addon) *core.Dispatcher {
	dispatcher := core.NewDispatcher()
	for idx, addon := range addons {
		dispatcher.Register(addon, idx*10)
	}
	return dispatcher
}
