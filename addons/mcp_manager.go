// MCPManager — dynamic load/unload of MCP servers.

package addons

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// MCPManager manages a chain of MCP servers — load/unload dynamically
// Supports both stdio and HTTP transports
type MCPManager struct {
	core.BaseAddon
	mu          sync.RWMutex
	addons      map[string]core.Addon
	registry    *core.ToolRegistry
	dispatcher  *core.Dispatcher
	permissions *PermissionStore
}

func NewMCPManager(dispatcher *core.Dispatcher, permissionsFile string) *MCPManager {
	return &MCPManager{
		addons:      make(map[string]core.Addon),
		registry:    core.NewToolRegistry(),
		dispatcher:  dispatcher,
		permissions: NewPermissionStore(permissionsFile),
	}
}

func (mgr *MCPManager) Name() string           { return "mcp-manager" }
func (mgr *MCPManager) Type() core.AddonType { return core.AddonMCP }

func (mgr *MCPManager) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnSessionStart, // inject tool registry into context
		core.OnToolRequest,  // route tool calls to correct MCP client
	}
}

func (mgr *MCPManager) Start() error { return nil }

func (mgr *MCPManager) Stop() error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for name, addon := range mgr.addons {
		addon.Stop()
		delete(mgr.addons, name)
	}
	return nil
}

func (mgr *MCPManager) Commands() []core.Command {
	return []core.Command{
		{Name: "mcp", Description: "manage MCP tool servers and permissions",
			Usage: "/mcp [list|load|unload|tools|permissions|permit]"},
	}
}

func (mgr *MCPManager) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return mgr.listServers()
	}

	switch parts[0] {
	case "load":
		if len(parts) < 3 {
			return "Usage: /mcp load <name> <command> [args...]\n       /mcp load <name> http <url>"
		}
		name := parts[1]
		if parts[2] == "http" {
			if len(parts) < 4 {
				return "Usage: /mcp load <name> http <url>"
			}
			return mgr.LoadHTTPServer(name, parts[3])
		}
		command := parts[2]
		cmdArgs := parts[3:]
		return mgr.LoadStdioServer(name, command, cmdArgs, nil)

	case "unload":
		if len(parts) < 2 {
			return "Usage: /mcp unload <name>"
		}
		return mgr.UnloadServer(parts[1])

	case "list":
		return mgr.listServers()

	case "tools":
		query := ""
		if len(parts) > 1 {
			query = strings.Join(parts[1:], " ")
		}
		return mgr.listTools(query)

	case "permissions", "perms":
		return mgr.permissions.Format()

	case "permit":
		if len(parts) < 3 {
			return "Usage: /mcp permit <tool|pattern> allow|deny|ask"
		}
		pattern := parts[1]
		perm, ok := parsePermission(parts[2])
		if !ok {
			return fmt.Sprintf("Unbekannte Permission: %s (allow|deny|ask)", parts[2])
		}
		mgr.permissions.Set(pattern, perm)
		return fmt.Sprintf("Permission gesetzt: %s → %s", pattern, perm)

	case "unpermit":
		if len(parts) < 2 {
			return "Usage: /mcp unpermit <tool|pattern>"
		}
		mgr.permissions.Remove(parts[1])
		return fmt.Sprintf("Permission entfernt: %s (Default gilt)", parts[1])

	default:
		return "Unknown: /mcp " + parts[0] + "\nUsage: /mcp [list|load|unload|tools|permissions|permit]"
	}
}

// LoadStdioServer uses a goroutine for Start() to prevent deadlock with terminal stdin.
func (mgr *MCPManager) LoadStdioServer(name, command string, args, env []string) string {
	mgr.unloadExisting(name)

	client := NewMCPClient(name, command, args, env, mgr.registry)

	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		ch <- result{err: client.Start()}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return fmt.Sprintf("Error loading %s: %v", name, r.err)
		}
	case <-time.After(15 * time.Second):
		client.Stop()
		return fmt.Sprintf("Timeout loading %s", name)
	}

	mgr.mu.Lock()
	mgr.addons[name] = client
	mgr.mu.Unlock()

	mgr.dispatcher.RegisterAt(client, 50, core.OnToolRequest)

	return fmt.Sprintf("Loaded %s (stdio): %d tools (%s %s)",
		name, len(client.tools), command, strings.Join(args, " "))
}

func (mgr *MCPManager) LoadHTTPServer(name, url string) string {
	mgr.unloadExisting(name)

	client := NewMCPHTTPClient(name, url, mgr.registry)
	if err := client.Start(); err != nil {
		return fmt.Sprintf("Error loading %s: %v", name, err)
	}

	mgr.mu.Lock()
	mgr.addons[name] = client
	mgr.mu.Unlock()

	mgr.dispatcher.RegisterAt(client, 50, core.OnToolRequest)

	return fmt.Sprintf("Loaded %s (HTTP): %d tools (%s)", name, client.ToolCount(), url)
}

func (mgr *MCPManager) UnloadServer(name string) string {
	mgr.mu.Lock()
	addon, ok := mgr.addons[name]
	if !ok {
		mgr.mu.Unlock()
		return fmt.Sprintf("Server %s not loaded", name)
	}
	delete(mgr.addons, name)
	mgr.mu.Unlock()

	addon.Stop()
	mgr.dispatcher.Unregister(addon.Name())

	return fmt.Sprintf("Unloaded %s", name)
}

func (mgr *MCPManager) unloadExisting(name string) {
	mgr.mu.Lock()
	if existing, ok := mgr.addons[name]; ok {
		existing.Stop()
		mgr.dispatcher.Unregister(existing.Name())
		delete(mgr.addons, name)
	}
	mgr.mu.Unlock()
}

func (mgr *MCPManager) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	switch hook {
	case core.OnSessionStart:
		ctx.Set(KeyToolRegistry, mgr.registry)
	case core.OnToolRequest:
		ctx.Set(KeyToolRegistry, mgr.registry)
	}
	return core.Result{}
}

func (mgr *MCPManager) Registry() *core.ToolRegistry {
	return mgr.registry
}

func (mgr *MCPManager) listServers() string {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if len(mgr.addons) == 0 {
		return "No MCP servers loaded.\n  /mcp load <name> http <url>\n  /mcp load <name> <command> [args...]"
	}

	var lines []string
	for name, addon := range mgr.addons {
		switch typed := addon.(type) {
		case *MCPClient:
			lines = append(lines, fmt.Sprintf("  %-15s %d tools (stdio: %s)", name, len(typed.tools), typed.command))
		case *MCPHTTPClient:
			lines = append(lines, fmt.Sprintf("  %-15s %d tools (HTTP: %s)", name, typed.ToolCount(), typed.baseURL))
		default:
			lines = append(lines, fmt.Sprintf("  %-15s [%s]", name, addon.Type()))
		}
	}
	return "MCP Servers:\n" + strings.Join(lines, "\n")
}

func (mgr *MCPManager) listTools(query string) string {
	tools := mgr.registry.All()
	if len(tools) == 0 {
		return "No tools available."
	}

	query = strings.ToLower(query)
	var lines []string
	for _, tool := range tools {
		if query != "" &&
			!strings.Contains(strings.ToLower(tool.Name), query) &&
			!strings.Contains(strings.ToLower(tool.Description), query) {
			continue
		}
		source := mgr.registry.Source(tool.Name)
		perm := mgr.permissions.Get(tool.Name)
		permIcon := "?"
		switch perm {
		case PermAllow:
			permIcon = "✓"
		case PermDeny:
			permIcon = "✗"
		}
		lines = append(lines, fmt.Sprintf("  %s %-28s [%s] %s", permIcon, tool.Name, source, tool.Description))
	}

	if len(lines) == 0 {
		return "No matching tools."
	}
	return fmt.Sprintf("Tools (%d)  ✓=allow ✗=deny ?=ask:\n%s", len(lines), strings.Join(lines, "\n"))
}

func (mgr *MCPManager) CheckPermission(toolName string) ToolPermission {
	return mgr.permissions.Get(toolName)
}

func (mgr *MCPManager) CallTool(toolName, argsJSON string) string {
	source := mgr.registry.Source(toolName)
	if source == "" {
		return fmt.Sprintf("error: tool %q not found", toolName)
	}

	mgr.mu.RLock()
	addon, ok := mgr.addons[source]
	mgr.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("error: server %q not loaded", source)
	}

	var args map[string]interface{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("error: invalid args JSON: %v", err)
		}
	}

	switch client := addon.(type) {
	case *MCPClient:
		result, err := client.callTool(toolName, args)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return result
	case *MCPHTTPClient:
		result, err := client.callTool(toolName, args)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return result
	}
	return fmt.Sprintf("error: server %q unsupported type", source)
}
