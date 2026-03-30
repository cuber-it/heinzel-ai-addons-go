// FileUploadAddon — @path syntax for file and image injection.

package addons

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

type FileUploadAddon struct {
	core.BaseAddon
	maxFileSize int64 // bytes
}

func NewFileUploadAddon(maxSizeMB int) *FileUploadAddon {
	if maxSizeMB <= 0 {
		maxSizeMB = 10
	}
	return &FileUploadAddon{
		maxFileSize: int64(maxSizeMB) * 1024 * 1024,
	}
}

func (addon *FileUploadAddon) Name() string           { return "file-upload" }
func (addon *FileUploadAddon) Type() core.AddonType { return core.AddonFilter }
func (addon *FileUploadAddon) Start() error            { return nil }
func (addon *FileUploadAddon) Stop() error             { return nil }

func (addon *FileUploadAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnInput}
}

func (addon *FileUploadAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "file", Description: "attach a file to the conversation",
			Usage: "/file <path>"},
	}
}

func (addon *FileUploadAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	path := strings.TrimSpace(args)
	if path == "" {
		return "Usage: /file <path>"
	}
	return addon.processFile(path, ctx)
}

func (addon *FileUploadAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	input := ctx.Input
	words := strings.Fields(input)
	var cleanWords []string
	filesProcessed := 0

	for _, word := range words {
		if strings.HasPrefix(word, "@") && len(word) > 1 {
			path := word[1:]
			if _, err := os.Stat(path); err == nil {
				addon.processFile(path, ctx)
				filesProcessed++
				continue
			}
		}
		if strings.HasPrefix(word, "file://") {
			path := strings.TrimPrefix(word, "file://")
			if _, err := os.Stat(path); err == nil {
				addon.processFile(path, ctx)
				filesProcessed++
				continue
			}
		}
		cleanWords = append(cleanWords, word)
	}

	if filesProcessed > 0 {
		ctx.Input = strings.Join(cleanWords, " ")
		thinking := core.GetThinking(ctx)
		if thinking != nil {
			thinking.AddStep("tool", fmt.Sprintf("%d file(s) attached", filesProcessed), "file-upload")
		}
	}

	return core.Result{}
}

func (addon *FileUploadAddon) processFile(path string, ctx *core.Context) string {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if info.Size() > addon.maxFileSize {
		return fmt.Sprintf("Error: file too large (%d MB, max %d MB)", info.Size()/1024/1024, addon.maxFileSize/1024/1024)
	}

	ext := strings.ToLower(filepath.Ext(path))

	switch {
	case isImageExt(ext):
		return addon.attachImage(path, ext, ctx)
	default:
		return addon.attachText(path, ctx)
	}
}

func (addon *FileUploadAddon) attachText(path string, ctx *core.Context) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", path, err)
	}

	name := filepath.Base(path)
	text := fmt.Sprintf("[File: %s]\n```\n%s\n```", name, string(content))

	ctx.Prompts.Add(core.LayerTurn, "file:"+name, text, 50)

	return fmt.Sprintf("Attached: %s (%d bytes)", name, len(content))
}

func (addon *FileUploadAddon) attachImage(path, ext string, ctx *core.Context) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", path, err)
	}

	name := filepath.Base(path)
	b64 := base64.StdEncoding.EncodeToString(content)
	mediaType := imageMediaType(ext)

	images, _ := ctx.Get(KeyAttachedImages)
	var imageList []map[string]string
	if existing, ok := images.([]map[string]string); ok {
		imageList = existing
	}
	imageList = append(imageList, map[string]string{
		"name":       name,
		"media_type": mediaType,
		"data":       b64,
	})
	ctx.Set(KeyAttachedImages, imageList)

	return fmt.Sprintf("Attached image: %s (%d KB, %s)", name, len(content)/1024, mediaType)
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
		return true
	}
	return false
}

func isTextExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".go", ".py", ".js", ".ts", ".yaml", ".yml",
		".json", ".xml", ".html", ".css", ".sh", ".bash", ".sql",
		".csv", ".log", ".conf", ".cfg", ".ini", ".toml",
		".rs", ".c", ".h", ".cpp", ".java", ".rb", ".pl", ".plg",
		".fth", ".pro", ".lisp", ".el":
		return true
	}
	return false
}

func imageMediaType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	}
	return "application/octet-stream"
}
