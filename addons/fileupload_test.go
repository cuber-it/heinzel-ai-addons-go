package addons

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewFileUploadAddon(test *testing.T) {
	addon := NewFileUploadAddon(10)
	if addon == nil {
		test.Fatal("NewFileUploadAddon returned nil")
	}
	if addon.Name() != "file-upload" {
		test.Errorf("expected name 'file-upload', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
}

func TestFileUploadAddonDefaultMaxSize(test *testing.T) {
	addon := NewFileUploadAddon(0)
	expectedSize := int64(10 * 1024 * 1024) // default 10 MB
	if addon.maxFileSize != expectedSize {
		test.Errorf("expected default max size %d, got %d", expectedSize, addon.maxFileSize)
	}
}

func TestFileUploadAddonDetectsAtPathSyntax(test *testing.T) {
	tmpDir := test.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello from file"), 0644)

	addon := NewFileUploadAddon(10)
	ctx := testContext()
	ctx.Input = "Lies das @" + testFile

	addon.Handle(core.OnInput, ctx)

	// Input should have the @path removed
	if strings.Contains(ctx.Input, "@") {
		test.Errorf("expected @path removed from input, got: %s", ctx.Input)
	}
	if !strings.Contains(ctx.Input, "Lies das") {
		test.Errorf("expected remaining text preserved, got: %s", ctx.Input)
	}

	// File content should be in prompt blocks
	blocks := ctx.Prompts.Blocks()
	foundFile := false
	for _, block := range blocks {
		if strings.Contains(block.Content, "Hello from file") {
			foundFile = true
		}
	}
	if !foundFile {
		test.Error("expected file content injected into prompt blocks")
	}
}

func TestFileUploadAddonReadsFileContent(test *testing.T) {
	tmpDir := test.TempDir()
	testFile := filepath.Join(tmpDir, "code.go")
	content := "package main\n\nfunc main() {}\n"
	os.WriteFile(testFile, []byte(content), 0644)

	addon := NewFileUploadAddon(10)
	ctx := testContext()

	result := addon.HandleCommand("file", testFile, ctx)
	if !strings.Contains(result, "Attached:") {
		test.Errorf("expected 'Attached:' in result, got: %s", result)
	}
	if !strings.Contains(result, "code.go") {
		test.Errorf("expected filename in result, got: %s", result)
	}
}

func TestFileUploadAddonDetectsImageFiles(test *testing.T) {
	if !isImageExt(".png") {
		test.Error("expected .png to be detected as image")
	}
	if !isImageExt(".jpg") {
		test.Error("expected .jpg to be detected as image")
	}
	if !isImageExt(".jpeg") {
		test.Error("expected .jpeg to be detected as image")
	}
	if !isImageExt(".gif") {
		test.Error("expected .gif to be detected as image")
	}
	if !isImageExt(".webp") {
		test.Error("expected .webp to be detected as image")
	}
	if isImageExt(".txt") {
		test.Error("expected .txt NOT to be detected as image")
	}
}

func TestFileUploadAddonImageAttachment(test *testing.T) {
	tmpDir := test.TempDir()
	// Write a fake PNG (just bytes, not a real image, but sufficient for testing)
	imgFile := filepath.Join(tmpDir, "test.png")
	os.WriteFile(imgFile, []byte("fake-png-data"), 0644)

	addon := NewFileUploadAddon(10)
	ctx := testContext()

	result := addon.HandleCommand("file", imgFile, ctx)
	if !strings.Contains(result, "Attached image:") {
		test.Errorf("expected 'Attached image:' in result, got: %s", result)
	}
	if !strings.Contains(result, "image/png") {
		test.Errorf("expected media type in result, got: %s", result)
	}

	// Check that image was stored in context state
	val, ok := ctx.Get(KeyAttachedImages)
	if !ok {
		test.Fatal("expected KeyAttachedImages to be set")
	}
	images, ok := val.([]map[string]string)
	if !ok || len(images) != 1 {
		test.Fatalf("expected 1 image attachment, got %v", val)
	}
	if images[0]["media_type"] != "image/png" {
		test.Errorf("expected media_type 'image/png', got %s", images[0]["media_type"])
	}
}

func TestFileUploadAddonMissingFile(test *testing.T) {
	addon := NewFileUploadAddon(10)
	ctx := testContext()

	result := addon.HandleCommand("file", "/nonexistent/file.txt", ctx)
	if !strings.Contains(result, "Error") {
		test.Errorf("expected error for missing file, got: %s", result)
	}
}

func TestFileUploadAddonImageMediaTypes(test *testing.T) {
	cases := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
		".xyz":  "application/octet-stream",
	}
	for ext, expected := range cases {
		result := imageMediaType(ext)
		if result != expected {
			test.Errorf("imageMediaType(%q) = %q, expected %q", ext, result, expected)
		}
	}
}
