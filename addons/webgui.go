// WebGUIBridge — web chat interface with Markdown, Mermaid, SSE streaming.

package addons

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// WebGUIBridge is an IOBridge that serves a web chat interface
type WebGUIBridge struct {
	core.BaseAddon
	port       int
	webFS      embed.FS
	loop       *core.Loop
	ctx        *core.Context
	mu         sync.Mutex
	sseClients map[chan string]bool
	sseMu      sync.Mutex
	uploadDirs []string // temp directories to clean up
	uploadMu   sync.Mutex
}

func NewWebGUIBridge(port int, webFS embed.FS) *WebGUIBridge {
	return &WebGUIBridge{
		port:       port,
		webFS:      webFS,
		sseClients: make(map[chan string]bool),
	}
}

func (gui *WebGUIBridge) Name() string           { return "webgui" }
func (gui *WebGUIBridge) Type() core.AddonType { return core.AddonTool }
func (gui *WebGUIBridge) Start() error            { return nil }
func (gui *WebGUIBridge) Stop() error {
	gui.uploadMu.Lock()
	defer gui.uploadMu.Unlock()
	for _, dir := range gui.uploadDirs {
		os.RemoveAll(dir)
	}
	gui.uploadDirs = nil
	return nil
}
func (gui *WebGUIBridge) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnSessionStart}
}

func (gui *WebGUIBridge) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	return core.Result{}
}

// Drive starts the HTTP server and blocks
func (gui *WebGUIBridge) Drive(loop *core.Loop) {
	gui.loop = loop
	gui.ctx = core.NewContext("web")

	core.InitThinking(gui.ctx, func(step core.ThinkingStep) {
		gui.broadcast("thinking", step.String())
	})

	gui.ctx.OnToken = func(token string) {
		gui.broadcast("token", token)
	}

	loop.Dispatcher.Dispatch(core.OnSessionStart, gui.ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", gui.handleChat)
	mux.HandleFunc("/api/upload", gui.handleUpload)
	mux.HandleFunc("/api/status", gui.handleStatus)
	mux.HandleFunc("/api/addon/", gui.handleAddonInfo)
	mux.HandleFunc("/api/events", gui.handleSSE)

	webSub, err := fs.Sub(gui.webFS, "web")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(webSub)))
	}

	addr := fmt.Sprintf(":%d", gui.port)
	fmt.Printf("Web GUI on http://localhost%s\n", addr)
	http.ListenAndServe(addr, mux)
}

// DriveBackground starts only the HTTP server without blocking
// Used when running alongside another IOBridge (e.g. Mattermost)
func (gui *WebGUIBridge) DriveBackground(loop *core.Loop) {
	gui.loop = loop
	gui.ctx = core.NewContext("web")

	core.InitThinking(gui.ctx, func(step core.ThinkingStep) {
		gui.broadcast("thinking", step.String())
	})
	gui.ctx.OnToken = func(token string) {
		gui.broadcast("token", token)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", gui.handleChat)
	mux.HandleFunc("/api/upload", gui.handleUpload)
	mux.HandleFunc("/api/status", gui.handleStatus)
	mux.HandleFunc("/api/addon/", gui.handleAddonInfo)
	mux.HandleFunc("/api/events", gui.handleSSE)

	webSub, err := fs.Sub(gui.webFS, "web")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(webSub)))
	}

	addr := fmt.Sprintf(":%d", gui.port)
	http.ListenAndServe(addr, mux) // blocks in goroutine
}

func (gui *WebGUIBridge) handleChat(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	json.NewDecoder(request.Body).Decode(&req)

	if strings.TrimSpace(req.Message) == "" {
		http.Error(writer, "empty message", http.StatusBadRequest)
		return
	}

	gui.broadcast("user", req.Message)

	gui.mu.Lock()
	output := gui.loop.Run(gui.ctx, req.Message)
	gui.mu.Unlock()

	gui.broadcast("done", "")

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]string{"response": output})
}

func (gui *WebGUIBridge) handleUpload(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, maxFileUploadBytes+1024)
	if err := request.ParseMultipartForm(maxFileUploadBytes); err != nil {
		http.Error(writer, "file too large (max 10MB)", http.StatusRequestEntityTooLarge)
		return
	}

	tmpDir, err := os.MkdirTemp("", "heinzel-upload-")
	if err != nil {
		http.Error(writer, "failed to create temp dir", http.StatusInternalServerError)
		return
	}

	gui.uploadMu.Lock()
	gui.uploadDirs = append(gui.uploadDirs, tmpDir)
	gui.uploadMu.Unlock()

	type fileInfo struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var files []fileInfo

	for _, headers := range request.MultipartForm.File {
		for _, header := range headers {
			if header.Size > maxFileUploadBytes {
				http.Error(writer, fmt.Sprintf("file %s too large", header.Filename), http.StatusRequestEntityTooLarge)
				return
			}

			src, err := header.Open()
			if err != nil {
				http.Error(writer, "failed to read file", http.StatusInternalServerError)
				return
			}

			safeName := filepath.Base(header.Filename)
			destPath := filepath.Join(tmpDir, safeName)

			dst, err := os.Create(destPath)
			if err != nil {
				src.Close()
				http.Error(writer, "failed to save file", http.StatusInternalServerError)
				return
			}

			_, err = io.Copy(dst, src)
			src.Close()
			dst.Close()
			if err != nil {
				http.Error(writer, "failed to write file", http.StatusInternalServerError)
				return
			}

			files = append(files, fileInfo{Name: safeName, Path: destPath})
		}
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]interface{}{"files": files})
}

func (gui *WebGUIBridge) handleStatus(writer http.ResponseWriter, request *http.Request) {
	gui.mu.Lock()
	status := map[string]interface{}{
		"session":  gui.ctx.SessionID,
		"messages": len(gui.ctx.Messages),
		"addons":   gui.loop.Dispatcher.ListAddons(),
	}
	if val, ok := gui.ctx.Get(KeyStrategy); ok {
		status["strategy"] = fmt.Sprintf("%v", val)
	}
	gui.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(status)
}

func (gui *WebGUIBridge) handleAddonInfo(writer http.ResponseWriter, request *http.Request) {
	name := strings.TrimPrefix(request.URL.Path, "/api/addon/")
	if name == "" {
		http.Error(writer, "missing addon name", http.StatusBadRequest)
		return
	}

	addon, ok := gui.loop.Dispatcher.GetAddon(name)
	if !ok {
		http.Error(writer, "addon not found", http.StatusNotFound)
		return
	}

	var hooks []string
	for hook := core.HookPoint(0); hook < core.HookCount; hook++ {
		for _, sub := range gui.loop.Dispatcher.HookSubscribers(hook) {
			if sub == name {
				hooks = append(hooks, hook.String())
			}
		}
	}

	var commands []map[string]string
	for _, cmd := range addon.Commands() {
		commands = append(commands, map[string]string{
			"name":        cmd.Name,
			"description": cmd.Description,
			"usage":       cmd.Usage,
		})
	}

	var capabilities map[string]interface{}
	if cp, ok := addon.(core.CapabilityProvider); ok {
		caps := cp.Capabilities()
		capabilities = map[string]interface{}{
			"streaming":     caps.Streaming,
			"tool_use":      caps.ToolUse,
			"vision":        caps.Vision,
			"audio":         caps.Audio,
			"context_window": caps.ContextWindow,
			"max_tokens":    caps.MaxTokens,
			"provider":      caps.ProviderName,
			"model":         caps.ModelName,
		}
	}

	info := map[string]interface{}{
		"name":     name,
		"type":     addon.Type().String(),
		"hooks":    hooks,
		"commands": commands,
		"status":   "running",
	}
	if capabilities != nil {
		info["capabilities"] = capabilities
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(info)
}

func (gui *WebGUIBridge) handleSSE(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming not supported", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 64)
	gui.sseMu.Lock()
	gui.sseClients[ch] = true
	gui.sseMu.Unlock()

	defer func() {
		gui.sseMu.Lock()
		delete(gui.sseClients, ch)
		gui.sseMu.Unlock()
	}()

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(writer, "data: %s\n\n", msg)
			flusher.Flush()
		case <-request.Context().Done():
			return
		}
	}
}

func (gui *WebGUIBridge) broadcast(eventType, content string) {
	msg, _ := json.Marshal(map[string]string{
		"type":    eventType,
		"content": content,
	})

	gui.sseMu.Lock()
	clients := make([]chan string, 0, len(gui.sseClients))
	for ch := range gui.sseClients {
		clients = append(clients, ch)
	}
	gui.sseMu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- string(msg):
		default:
		}
	}
}
