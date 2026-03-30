// MCPClient — MCP server connection via stdio transport.

package addons

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const maxMessageBytes = 1024 * 1024

// MCPClient connects to an MCP server via stdio and exposes its tools
type MCPClient struct {
	core.BaseAddon
	name       string
	command    string
	args       []string
	env        []string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	mu         sync.Mutex
	requestID  atomic.Int64
	tools      []core.Tool
	registry   *core.ToolRegistry
	responses  chan jsonRPCResponse // background reader pushes here
}

func NewMCPClient(name, command string, args []string, env []string, registry *core.ToolRegistry) *MCPClient {
	return &MCPClient{
		name:     name,
		command:  command,
		args:     args,
		env:      env,
		registry: registry,
	}
}

func (mcp *MCPClient) Name() string           { return "mcp:" + mcp.name }
func (mcp *MCPClient) Type() core.AddonType { return core.AddonMCP }

func (mcp *MCPClient) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnToolRequest,
	}
}

func (mcp *MCPClient) Commands() []core.Command {
	return []core.Command{
		{Name: "tools", Description: "list available MCP tools", Usage: "/tools [search]"},
	}
}

func (mcp *MCPClient) HandleCommand(cmd, args string, ctx *core.Context) string {
	if args == "" {
		var lines []string
		for _, tool := range mcp.tools {
			lines = append(lines, fmt.Sprintf("  %-30s %s", tool.Name, tool.Description))
		}
		if len(lines) == 0 {
			return "No tools available."
		}
		return fmt.Sprintf("Tools from %s (%d):\n%s", mcp.name, len(lines), strings.Join(lines, "\n"))
	}
	// Search
	query := strings.ToLower(args)
	var lines []string
	for _, tool := range mcp.tools {
		if strings.Contains(strings.ToLower(tool.Name), query) ||
			strings.Contains(strings.ToLower(tool.Description), query) {
			lines = append(lines, fmt.Sprintf("  %-30s %s", tool.Name, tool.Description))
		}
	}
	if len(lines) == 0 {
		return "No matching tools."
	}
	return strings.Join(lines, "\n")
}

func (mcp *MCPClient) Start() error {
	mcp.cmd = exec.Command(mcp.command, mcp.args...)
	if len(mcp.env) > 0 {
		mcp.cmd.Env = append(mcp.cmd.Environ(), mcp.env...)
	}

	setProcAttr(mcp.cmd)
	mcp.cmd.Stderr = io.Discard

	var err error
	mcp.stdin, err = mcp.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := mcp.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	mcp.stdout = bufio.NewReaderSize(stdoutPipe, maxMessageBytes)

	if err := mcp.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", mcp.command, err)
	}

	mcp.responses = make(chan jsonRPCResponse, 16)
	go mcp.readLoop()

	if err := mcp.initialize(); err != nil {
		mcp.cmd.Process.Kill()
		return fmt.Errorf("init %s: %w", mcp.name, err)
	}

	if err := mcp.discoverTools(); err != nil {
		mcp.cmd.Process.Kill()
		return fmt.Errorf("discover tools %s: %w", mcp.name, err)
	}

	return nil
}

func (mcp *MCPClient) Stop() error {
	if mcp.stdin != nil {
		mcp.stdin.Close()
	}

	if mcp.cmd != nil && mcp.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- mcp.cmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			mcp.cmd.Process.Kill()
			<-done // reap the zombie
		}
	}

	if mcp.responses != nil {
		for range mcp.responses {
		}
	}

	return nil
}

func (mcp *MCPClient) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if len(ctx.ToolCalls) == 0 {
		return core.Result{}
	}

	for idx := range ctx.ToolCalls {
		call := &ctx.ToolCalls[idx]
		// Only handle tools from this MCP server
		if mcp.registry.Source(call.Name) != mcp.Name() {
			continue
		}

		thinking := core.GetThinking(ctx)
		if thinking != nil {
			thinking.AddStep("tool", fmt.Sprintf("calling %s via %s", call.Name, mcp.name), "mcp")
		}

		result, err := mcp.callTool(call.Name, call.Args)
		if err != nil {
			call.Result = fmt.Sprintf("Error: %v", err)
			ctx.Error = err
		} else {
			call.Result = result
		}

		if thinking != nil {
			thinking.AddStep("tool", fmt.Sprintf("%s returned (%d chars)", call.Name, len(call.Result)), "mcp")
		}
	}

	return core.Result{}
}

// === JSON-RPC over stdio ===

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int64      `json:"id,omitempty"` // nil for notifications
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (mcp *MCPClient) readLoop() {
	for {
		line, err := mcp.stdout.ReadString('\n')
		if err != nil {
			close(mcp.responses)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		mcp.responses <- resp
	}
}

func (mcp *MCPClient) send(method string, params interface{}) (json.RawMessage, error) {
	mcp.mu.Lock()
	defer mcp.mu.Unlock()

	reqID := mcp.requestID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      &reqID,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if _, err := mcp.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	for {
		select {
		case resp, ok := <-mcp.responses:
			if !ok {
				return nil, fmt.Errorf("connection closed")
			}
			if resp.ID != reqID {
				continue
			}
			if resp.Error != nil {
				return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("timeout waiting for %s response", method)
		}
	}
}

func (mcp *MCPClient) initialize() error {
	_, err := mcp.send("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "neo-heinzel",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}

	mcp.mu.Lock()
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(notif)
	fmt.Fprintf(mcp.stdin, "%s\n", data)
	mcp.mu.Unlock()

	return nil
}

func (mcp *MCPClient) discoverTools() error {
	result, err := mcp.send("tools/list", nil)
	if err != nil {
		return err
	}

	var toolsResp struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &toolsResp); err != nil {
		return err
	}

	mcp.tools = nil
	for _, tool := range toolsResp.Tools {
		mcpTool := core.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		}
		mcp.tools = append(mcp.tools, mcpTool)
		mcp.registry.Register(mcpTool, mcp.Name())
	}

	return nil
}

func (mcp *MCPClient) callTool(name string, args map[string]interface{}) (string, error) {
	result, err := mcp.send("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var callResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &callResp); err != nil {
		return string(result), nil
	}

	var texts []string
	for _, block := range callResp.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}
