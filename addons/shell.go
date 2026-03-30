// Shell Addon — shell execution and file operations for Heinzel Assistant.
// All operations go through the ExecutionGuard before executing.

package addons

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

const shellTimeout = 30 * time.Second

// ShellAddon provides shell execution and file operations
type ShellAddon struct {
	core.BaseAddon
	guard *ExecutionGuard
}

func NewShellAddon(guard *ExecutionGuard) *ShellAddon {
	return &ShellAddon{
		guard: guard,
	}
}

func (addon *ShellAddon) Name() string         { return "shell" }
func (addon *ShellAddon) Type() core.AddonType { return core.AddonTool }
func (addon *ShellAddon) Start() error         { return nil }
func (addon *ShellAddon) Stop() error          { return nil }

func (addon *ShellAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnToolRequest}
}

func (addon *ShellAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "shell", Description: "execute a shell command", Usage: "/shell <command>"},
	}
}

func (addon *ShellAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	if args == "" {
		return "Usage: /shell <command>"
	}
	return addon.execShell(args)
}

func (addon *ShellAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook != core.OnToolRequest {
		return core.Result{}
	}

	for index := range ctx.ToolCalls {
		call := &ctx.ToolCalls[index]
		if call.Result != "" {
			continue
		}

		switch call.Name {
		case "shell_exec":
			command, _ := call.Args["command"].(string)
			if command == "" {
				call.Result = "error: missing 'command' argument"
				continue
			}
			call.Result = addon.execShell(command)

		case "file_read":
			path, _ := call.Args["path"].(string)
			if path == "" {
				call.Result = "error: missing 'path' argument"
				continue
			}
			call.Result = addon.fileRead(path)

		case "file_write":
			path, _ := call.Args["path"].(string)
			content, _ := call.Args["content"].(string)
			if path == "" {
				call.Result = "error: missing 'path' argument"
				continue
			}
			call.Result = addon.fileWrite(path, content)

		case "file_list":
			path, _ := call.Args["path"].(string)
			if path == "" {
				path = "."
			}
			call.Result = addon.fileList(path)

		case "file_delete":
			path, _ := call.Args["path"].(string)
			if path == "" {
				call.Result = "error: missing 'path' argument"
				continue
			}
			call.Result = addon.fileDelete(path)
		}
	}

	return core.Result{}
}

func (addon *ShellAddon) RegisterTools(registry *core.ToolRegistry) {
	registry.Register(core.Tool{
		Name:        "shell_exec",
		Description: "Execute a shell command and return stdout+stderr",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			"required": []string{"command"},
		},
	}, "shell")

	registry.Register(core.Tool{
		Name:        "file_read",
		Description: "Read the contents of a file",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read",
				},
			},
			"required": []string{"path"},
		},
	}, "shell")

	registry.Register(core.Tool{
		Name:        "file_write",
		Description: "Write content to a file (creates or overwrites)",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Content to write",
				},
			},
			"required": []string{"path", "content"},
		},
	}, "shell")

	registry.Register(core.Tool{
		Name:        "file_list",
		Description: "List contents of a directory",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Directory path (default: current directory)",
				},
			},
		},
	}, "shell")

	registry.Register(core.Tool{
		Name:        "file_delete",
		Description: "Delete a file (requires approval)",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to delete",
				},
			},
			"required": []string{"path"},
		},
	}, "shell")
}

// --- Operations ---

func (addon *ShellAddon) execShell(command string) string {
	ok, reason := addon.guard.Check("shell: "+command, ActionExecute)
	if !ok {
		return "guard: " + reason
	}

	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[Shell] executing: %s (timeout: %s)", command, shellTimeout)

	cmd := exec.CommandContext(ctx, defaultShell(), shellFlag(), command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start)

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("[stderr] ")
		result.WriteString(stderr.String())
	}
	if err != nil {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		if ctx.Err() == context.DeadlineExceeded {
			result.WriteString(fmt.Sprintf("[timeout] command exceeded %s limit after %s", shellTimeout, elapsed.Round(time.Millisecond)))
			log.Printf("[Shell] TIMEOUT after %s: %s", elapsed.Round(time.Millisecond), command)
		} else {
			result.WriteString(fmt.Sprintf("[exit] %v", err))
		}
	}

	if elapsed > 5*time.Second {
		log.Printf("[Shell] slow command (%s): %s", elapsed.Round(time.Millisecond), command)
	}

	output := result.String()
	if output == "" {
		output = "(no output)"
	}
	return output
}

func (addon *ShellAddon) fileRead(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("read: "+absPath, ActionRead)
	if !ok {
		return "guard: " + reason
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(data)
}

func (addon *ShellAddon) fileWrite(path, content string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("write: "+absPath, ActionWrite)
	if !ok {
		return "guard: " + reason
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("error creating directory: %v", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return fmt.Sprintf("written: %s (%d bytes)", absPath, len(content))
}

func (addon *ShellAddon) fileList(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("list: "+absPath, ActionRead)
	if !ok {
		return "guard: " + reason
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var lines []string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			lines = append(lines, fmt.Sprintf("  %-40s (error)", entry.Name()))
			continue
		}
		suffix := ""
		if entry.IsDir() {
			suffix = "/"
		}
		lines = append(lines, fmt.Sprintf("  %-40s %8d  %s",
			entry.Name()+suffix, info.Size(), info.ModTime().Format("2006-01-02 15:04")))
	}

	if len(lines) == 0 {
		return fmt.Sprintf("%s: (empty)", absPath)
	}
	return fmt.Sprintf("%s:\n%s", absPath, strings.Join(lines, "\n"))
}

func (addon *ShellAddon) fileDelete(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("delete: "+absPath, ActionDelete)
	if !ok {
		return "guard: " + reason
	}

	if err := os.Remove(absPath); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return fmt.Sprintf("deleted: %s", absPath)
}
