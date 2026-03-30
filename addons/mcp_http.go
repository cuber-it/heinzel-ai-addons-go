// MCPHTTPClient — MCP server connection via Streamable HTTP transport.

package addons

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// MCPHTTPClient connects to an MCP server via HTTP (Streamable HTTP transport)
// Protocol: POST to endpoint, JSON-RPC in body, SSE or JSON response
type MCPHTTPClient struct {
	core.BaseAddon
	name      string
	baseURL   string
	client    *http.Client
	mu        sync.Mutex
	requestID atomic.Int64
	tools     []core.Tool
	registry  *core.ToolRegistry
	sessionID string
}

func NewMCPHTTPClient(name, baseURL string, registry *core.ToolRegistry) *MCPHTTPClient {
	return &MCPHTTPClient{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
		registry: registry,
	}
}

func (mcp *MCPHTTPClient) Name() string           { return "mcp:" + mcp.name }
func (mcp *MCPHTTPClient) Type() core.AddonType { return core.AddonMCP }
func (mcp *MCPHTTPClient) Stop() error             { return nil }

func (mcp *MCPHTTPClient) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnToolRequest}
}

func (mcp *MCPHTTPClient) Start() error {
	if err := mcp.initialize(); err != nil {
		return fmt.Errorf("init %s: %w", mcp.name, err)
	}
	if err := mcp.discoverTools(); err != nil {
		return fmt.Errorf("discover %s: %w", mcp.name, err)
	}
	return nil
}

func (mcp *MCPHTTPClient) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if len(ctx.ToolCalls) == 0 {
		return core.Result{}
	}

	for idx := range ctx.ToolCalls {
		call := &ctx.ToolCalls[idx]
		if mcp.registry.Source(call.Name) != mcp.Name() {
			continue
		}

		thinking := core.GetThinking(ctx)
		if thinking != nil {
			thinking.AddStep("tool", fmt.Sprintf("calling %s via %s (HTTP)", call.Name, mcp.name), "mcp")
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

// === HTTP JSON-RPC ===

func (mcp *MCPHTTPClient) send(method string, params interface{}) (json.RawMessage, error) {
	mcp.mu.Lock()
	defer mcp.mu.Unlock()

	reqID := mcp.requestID.Add(1)

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", mcp.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if mcp.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", mcp.sessionID)
	}

	resp, err := mcp.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		mcp.sessionID = sid
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var rpcResp struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (mcp *MCPHTTPClient) notify(method string, params interface{}) error {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequest("POST", mcp.baseURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if mcp.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", mcp.sessionID)
	}

	resp, err := mcp.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (mcp *MCPHTTPClient) initialize() error {
	_, err := mcp.send("initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "neo-heinzel",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}

	return mcp.notify("notifications/initialized", nil)
}

func (mcp *MCPHTTPClient) discoverTools() error {
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

func (mcp *MCPHTTPClient) callTool(name string, args map[string]interface{}) (string, error) {
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

func (mcp *MCPHTTPClient) ToolCount() int {
	return len(mcp.tools)
}
