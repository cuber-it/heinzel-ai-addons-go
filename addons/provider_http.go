// HTTPProvider — the only LLM provider, talks to external provider service.

package addons

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// HTTPProvider is the ONLY provider in neo-heinzel
// All LLM communication goes through an external provider service
// Cost tracking, logging, monitoring — all in the provider service
type HTTPProvider struct {
	core.BaseAddon
	mu      sync.RWMutex
	name    string
	baseURL string
	model   string
	client  *http.Client
	healthy bool
	caps    *core.ProviderCapabilities
}

// BaseURL returns the provider service URL (for audio/speak endpoints etc.)
func (prov *HTTPProvider) BaseURL() string { return prov.baseURL }

func NewHTTPProvider(name, baseURL, model string) *HTTPProvider {
	return &HTTPProvider{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (prov *HTTPProvider) Name() string           { return prov.name }
func (prov *HTTPProvider) Type() core.AddonType { return core.AddonProvider }

func (prov *HTTPProvider) Start() error {
	resp, err := prov.client.Get(prov.baseURL + "/health")
	if err != nil {
		prov.healthy = false
		return fmt.Errorf("provider %s not reachable: %w", prov.name, err)
	}
	resp.Body.Close()
	prov.healthy = resp.StatusCode == 200

	prov.fetchCapabilities()
	return nil
}

func (prov *HTTPProvider) Stop() error { return nil }

func (prov *HTTPProvider) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnLLMCall}
}

func (prov *HTTPProvider) Capabilities() core.ProviderCapabilities {
	if prov.caps != nil {
		return *prov.caps
	}
	return core.ProviderCapabilities{
		Streaming:     true,
		ToolUse:       true,
		Vision:        true,
		ProviderName:  prov.name,
		ModelName:     prov.model,
		ContextWindow: 128000,
	}
}

func (prov *HTTPProvider) Commands() []core.Command {
	return []core.Command{
		{Name: "provider", Description: "LLM provider control",
			Usage: "provider [status|model <name>|models|switch <url>|health]"},
	}
}

func (prov *HTTPProvider) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = parts[0]
	}

	switch sub {
	case "", "status":
		prov.mu.RLock()
		defer prov.mu.RUnlock()
		status := "unhealthy"
		if prov.healthy {
			status = "healthy"
		}
		return fmt.Sprintf("Provider: %s (%s)\nModel: %s\nURL: %s",
			prov.name, status, prov.model, prov.baseURL)

	case "model":
		if len(parts) < 2 {
			return "Usage: provider model <name>"
		}
		prov.mu.Lock()
		prov.model = parts[1]
		prov.mu.Unlock()
		return fmt.Sprintf("Model: %s", parts[1])

	case "models":
		return prov.fetchModels()

	case "switch":
		if len(parts) < 2 {
			return "Usage: provider switch <url>"
		}
		prov.mu.Lock()
		prov.baseURL = strings.TrimRight(parts[1], "/")
		prov.mu.Unlock()
		if err := prov.Start(); err != nil {
			return fmt.Sprintf("Switch failed: %v", err)
		}
		return fmt.Sprintf("Provider switched to: %s", parts[1])

	case "health":
		resp, err := prov.client.Get(prov.baseURL + "/health")
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	return "Usage: provider [status|model <name>|models|switch <url>|health]"
}

func (prov *HTTPProvider) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	prov.mu.RLock()
	baseURL := prov.baseURL
	model := prov.model
	prov.mu.RUnlock()

	thinking := core.GetThinking(ctx)
	if thinking != nil {
		thinking.AddStep("system", fmt.Sprintf("calling %s (%s)", prov.name, model), "provider")
	}

	messages := prov.buildChatMessages(ctx)
	reqBody, shouldStream := prov.buildRequestBody(ctx, messages, model)

	jsonBody, _ := json.Marshal(reqBody)
	startTime := time.Now()

	resp, err := prov.client.Post(baseURL+"/chat", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		prov.mu.Lock()
		prov.healthy = false
		prov.mu.Unlock()
		return core.Result{Error: fmt.Errorf("provider error: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return core.Result{Error: fmt.Errorf("provider %d: %s", resp.StatusCode, string(body))}
	}

	if shouldStream {
		return prov.handleStreaming(resp, ctx, thinking, startTime)
	}
	return prov.handleNonStreaming(resp, ctx, thinking, startTime)
}

func (prov *HTTPProvider) buildChatMessages(ctx *core.Context) []map[string]interface{} {
	var messages []map[string]interface{}
	for _, msg := range ctx.Messages {
		if msg.Role == "system" {
			continue
		}
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	if images, ok := ctx.Get(KeyAttachedImages); ok {
		if imageList, ok := images.([]map[string]string); ok && len(imageList) > 0 && len(messages) > 0 {
			lastIdx := len(messages) - 1
			if messages[lastIdx]["role"] == "user" {
				var contentParts []map[string]interface{}
				contentParts = append(contentParts, map[string]interface{}{
					"type": "text", "text": messages[lastIdx]["content"],
				})
				for _, img := range imageList {
					contentParts = append(contentParts, map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": "data:" + img["media_type"] + ";base64," + img["data"]},
					})
				}
				messages[lastIdx]["content"] = contentParts
			}
			ctx.Set(KeyAttachedImages, nil)
		}
	}
	return messages
}

func (prov *HTTPProvider) buildRequestBody(ctx *core.Context, messages []map[string]interface{}, model string) (map[string]interface{}, bool) {
	hasImages := false
	for _, msg := range messages {
		if _, ok := msg["content"].([]map[string]interface{}); ok {
			hasImages = true
			break
		}
	}
	hasTools := false
	if val, ok := ctx.Get(KeyToolRegistry); ok {
		if reg, ok := val.(*core.ToolRegistry); ok && reg.Count() > 0 {
			hasTools = true
		}
	}

	shouldStream := !hasImages && !hasTools && ctx.OnToken != nil

	reqBody := map[string]interface{}{
		"messages": messages,
		"model":    model,
		"stream":   shouldStream,
	}
	if ctx.SystemPrompt != "" {
		reqBody["system"] = ctx.SystemPrompt
	}
	if val, ok := ctx.Get(KeyMaxOutputTokens); ok {
		if maxTokens, ok := val.(int); ok && maxTokens > 0 {
			reqBody["max_tokens"] = maxTokens
		}
	}
	// Native reasoning — tell provider to use model's built-in thinking
	if val, ok := ctx.Get(KeyNativeReasoning); ok {
		if native, ok := val.(bool); ok && native {
			reqBody["reasoning_effort"] = "medium"
		}
	}
	return reqBody, shouldStream
}

func (prov *HTTPProvider) handleStreaming(resp *http.Response, ctx *core.Context, thinking *core.ThinkingStream, startTime time.Time) core.Result {
	var fullOutput strings.Builder
	var reasoningBuf strings.Builder
	chunks := 0
	var usageIn, usageOut int

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk struct {
			Type    string `json:"type"`
			Content string `json:"content"`
			Usage   *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		switch chunk.Type {
		case "content_delta":
			chunks++
			fullOutput.WriteString(chunk.Content)
			if ctx.OnToken != nil {
				ctx.OnToken(chunk.Content)
			}
		case "reasoning_delta":
			reasoningBuf.WriteString(chunk.Content)
		case "usage":
			if chunk.Usage != nil {
				usageIn = chunk.Usage.InputTokens
				usageOut = chunk.Usage.OutputTokens
			}
		case "done":
			// done
		}
	}

	elapsed := time.Since(startTime)

	// Native reasoning from provider (o-models, Claude extended_thinking)
	if reasoningBuf.Len() > 0 && thinking != nil {
		thinking.AddStep("reason", reasoningBuf.String(), "llm")
	}

	raw := fullOutput.String()
	ctx.Output = prov.extractThinking(raw, thinking)
	if ctx.OnToken != nil {
		ctx.OnToken("\n")
	}
	ctx.Set(KeyOutputStreamed, true)
	ctx.Set(KeyLLMTokensIn, usageIn)
	ctx.Set(KeyLLMTokensOut, usageOut)

	if thinking != nil {
		thinking.AddStep("think", fmt.Sprintf("streamed %d chunks (%s, %d+%d tokens)",
			chunks, elapsed.Round(time.Millisecond), usageIn, usageOut), "provider")
	}
	return core.Result{}
}

// extractThinking splits thinking markers (### or <think> tags) from the actual answer.
func (prov *HTTPProvider) extractThinking(raw string, thinking *core.ThinkingStream) string {
	thinkMarker := "### Überlegungen"
	answerMarker := "### Antwort"

	thinkIdx := strings.Index(raw, thinkMarker)
	answerIdx := strings.Index(raw, answerMarker)

	if thinkIdx >= 0 && answerIdx > thinkIdx {
		thinkContent := strings.TrimSpace(raw[thinkIdx+len(thinkMarker) : answerIdx])
		answerContent := strings.TrimSpace(raw[answerIdx+len(answerMarker):])

		if thinking != nil && thinkContent != "" {
			thinking.AddStep("reason", thinkContent, "llm")
		}
		return answerContent
	}

	openIdx := strings.Index(raw, "<think>")
	closeIdx := strings.Index(raw, "</think>")

	if openIdx >= 0 && closeIdx > openIdx {
		thinkContent := strings.TrimSpace(raw[openIdx+7 : closeIdx])
		answerContent := strings.TrimSpace(raw[:openIdx] + raw[closeIdx+8:])

		if thinking != nil && thinkContent != "" {
			thinking.AddStep("reason", thinkContent, "llm")
		}
		return answerContent
	}

	return raw
}

func (prov *HTTPProvider) handleNonStreaming(resp *http.Response, ctx *core.Context, thinking *core.ThinkingStream, startTime time.Time) core.Result {
	body, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(startTime)

	var apiResp struct {
		Content   string `json:"content"`
		Reasoning string `json:"reasoning"` // native reasoning (o-models, Claude)
		ToolCalls []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"tool_calls"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return core.Result{Error: fmt.Errorf("unmarshal response: %w", err)}
	}

	if apiResp.Reasoning != "" && thinking != nil {
		thinking.AddStep("reason", apiResp.Reasoning, "llm")
	}

	ctx.Output = prov.extractThinking(apiResp.Content, thinking)
	ctx.Set(KeyLLMTokensIn, apiResp.Usage.InputTokens)
	ctx.Set(KeyLLMTokensOut, apiResp.Usage.OutputTokens)

	for _, tc := range apiResp.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			continue
		}
		ctx.ToolCalls = append(ctx.ToolCalls, core.ToolCall{
			ID: tc.ID, Name: tc.Name, Args: args,
		})
	}

	if thinking != nil {
		thinking.AddStep("think", fmt.Sprintf("response (%s, %d+%d tokens)",
			elapsed.Round(time.Millisecond), apiResp.Usage.InputTokens, apiResp.Usage.OutputTokens), "provider")
	}
	return core.Result{}
}

func (prov *HTTPProvider) fetchModels() string {
	resp, err := prov.client.Get(prov.baseURL + "/models")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func (prov *HTTPProvider) fetchCapabilities() {
	resp, err := prov.client.Get(prov.baseURL + "/models")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var models struct {
		Provider string   `json:"provider"`
		Default  string   `json:"default"`
		Models   []string `json:"models"`
	}
	if err := json.Unmarshal(body, &models); err != nil {
		return
	}

	if models.Provider != "" {
		prov.mu.Lock()
		prov.name = models.Provider
		if prov.model == "" {
			prov.model = models.Default
		}
		prov.caps = &core.ProviderCapabilities{
			Streaming:     true,
			ToolUse:       true,
			Vision:        true,
			ProviderName:  models.Provider,
			ModelName:     prov.model,
			ContextWindow: 128000,
		}
		prov.mu.Unlock()
	}
}
