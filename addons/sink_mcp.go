// MCPSink — compaction persistence via MCP tool calls.

package addons

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const mcpManagerAddonName = "mcp-manager"

// MCPSink persists compacted knowledge via MCP tool calls.
// Works with any MCP server that provides the right tools —
// vault (vault_write), prolog (prolog_assert), or custom.
type MCPSink struct {
	dispatcher *core.Dispatcher
	serverName string // MCP server name (e.g. "vault", "prolog")

	// Tool names — configured per backend
	factTool    string // e.g. "vault_write" or "prolog_assert"
	summaryTool string // e.g. "vault_write" or "prolog_assert"
}

// NewMCPSink creates a sink that calls MCP tools for persistence
func NewMCPSink(dispatcher *core.Dispatcher, serverName, factTool, summaryTool string) *MCPSink {
	return &MCPSink{
		dispatcher:  dispatcher,
		serverName:  serverName,
		factTool:    factTool,
		summaryTool: summaryTool,
	}
}

// StoreFact calls the configured MCP tool to persist a fact
func (sink *MCPSink) StoreFact(key, value, source string) error {
	args := map[string]interface{}{
		"key":    key,
		"value":  value,
		"source": source,
		"date":   time.Now().Format("2006-01-02"),
	}
	return sink.callTool(sink.factTool, args)
}

// StoreSummary calls the configured MCP tool to persist a summary
func (sink *MCPSink) StoreSummary(sessionID, summary string, timestamp time.Time) error {
	args := map[string]interface{}{
		"session_id": sessionID,
		"summary":    summary,
		"date":       timestamp.Format("2006-01-02"),
	}
	return sink.callTool(sink.summaryTool, args)
}

func (sink *MCPSink) callTool(toolName string, args map[string]interface{}) error {
	if sink.dispatcher == nil {
		return fmt.Errorf("no dispatcher")
	}

	// Find the MCP manager
	addon, ok := sink.dispatcher.GetAddon(mcpManagerAddonName)
	if !ok {
		return fmt.Errorf("%s not found", mcpManagerAddonName)
	}

	manager, ok := addon.(*MCPManager)
	if !ok {
		return fmt.Errorf("%s wrong type", mcpManagerAddonName)
	}

	argsJSON, _ := json.Marshal(args)
	result := manager.CallTool(toolName, string(argsJSON))
	if result == "" {
		return fmt.Errorf("tool %q returned empty", toolName)
	}
	return nil
}
