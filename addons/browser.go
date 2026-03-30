// Browser Addon — headless browser control via go-rod/rod.
// Lazy initialization: browser only starts when first used.

package addons

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const browserTimeout = 30 * time.Second

type BrowserAddon struct {
	core.BaseAddon
	guard   *ExecutionGuard
	mu      sync.Mutex
	browser *rod.Browser
	page    *rod.Page
}

func NewBrowserAddon(guard *ExecutionGuard) *BrowserAddon {
	return &BrowserAddon{
		guard: guard,
	}
}

func (addon *BrowserAddon) Name() string         { return "browser" }
func (addon *BrowserAddon) Type() core.AddonType { return core.AddonTool }
func (addon *BrowserAddon) Start() error         { return nil } // lazy init
func (addon *BrowserAddon) Stop() error          { return addon.closeBrowser() }

func (addon *BrowserAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnToolRequest}
}

func (addon *BrowserAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "browser", Description: "browser control", Usage: "/browser open <url>\n  /browser screenshot\n  /browser close"},
	}
}

func (addon *BrowserAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "Usage: /browser open <url> | screenshot | close"
	}

	switch parts[0] {
	case "open":
		if len(parts) < 2 {
			return "Usage: /browser open <url>"
		}
		return addon.browserOpen(parts[1])
	case "screenshot":
		return addon.browserScreenshot()
	case "close":
		return addon.browserClose()
	default:
		return "Usage: /browser open <url> | screenshot | close"
	}
}

func (addon *BrowserAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook != core.OnToolRequest {
		return core.Result{}
	}

	for index := range ctx.ToolCalls {
		call := &ctx.ToolCalls[index]
		if call.Result != "" {
			continue
		}

		switch call.Name {
		case "browser_open":
			url, _ := call.Args["url"].(string)
			if url == "" {
				call.Result = "error: missing 'url' argument"
				continue
			}
			call.Result = addon.browserOpen(url)

		case "browser_screenshot":
			call.Result = addon.browserScreenshot()

		case "browser_click":
			selector, _ := call.Args["selector"].(string)
			if selector == "" {
				call.Result = "error: missing 'selector' argument"
				continue
			}
			call.Result = addon.browserClick(selector)

		case "browser_type":
			selector, _ := call.Args["selector"].(string)
			text, _ := call.Args["text"].(string)
			if selector == "" || text == "" {
				call.Result = "error: missing 'selector' or 'text' argument"
				continue
			}
			call.Result = addon.browserType(selector, text)

		case "browser_content":
			call.Result = addon.browserContent()

		case "browser_close":
			call.Result = addon.browserClose()
		}
	}

	return core.Result{}
}

func (addon *BrowserAddon) RegisterTools(registry *core.ToolRegistry) {
	registry.Register(core.Tool{
		Name:        "browser_open",
		Description: "Open a URL in the headless browser",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "URL to open",
				},
			},
			"required": []string{"url"},
		},
	}, "browser")

	registry.Register(core.Tool{
		Name:        "browser_screenshot",
		Description: "Take a screenshot of the current page (returns base64 PNG)",
		Parameters: map[string]interface{}{
			"type": "object",
		},
	}, "browser")

	registry.Register(core.Tool{
		Name:        "browser_click",
		Description: "Click an element by CSS selector",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"selector": map[string]interface{}{
					"type":        "string",
					"description": "CSS selector of the element to click",
				},
			},
			"required": []string{"selector"},
		},
	}, "browser")

	registry.Register(core.Tool{
		Name:        "browser_type",
		Description: "Type text into an input element",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"selector": map[string]interface{}{
					"type":        "string",
					"description": "CSS selector of the input element",
				},
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to type",
				},
			},
			"required": []string{"selector", "text"},
		},
	}, "browser")

	registry.Register(core.Tool{
		Name:        "browser_content",
		Description: "Get the text content of the current page",
		Parameters: map[string]interface{}{
			"type": "object",
		},
	}, "browser")

	registry.Register(core.Tool{
		Name:        "browser_close",
		Description: "Close the browser",
		Parameters: map[string]interface{}{
			"type": "object",
		},
	}, "browser")
}

// --- Operations ---

func (addon *BrowserAddon) ensureBrowser() error {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	if addon.browser != nil {
		return nil
	}

	path, found := launcher.LookPath()
	if !found {
		return fmt.Errorf("Chrome/Chromium not found")
	}

	controlURL, err := launcher.New().Bin(path).Headless(true).Launch()
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return fmt.Errorf("failed to connect to browser: %w", err)
	}

	addon.browser = browser
	return nil
}

func (addon *BrowserAddon) ensurePage() (*rod.Page, error) {
	if err := addon.ensureBrowser(); err != nil {
		return nil, err
	}

	addon.mu.Lock()
	defer addon.mu.Unlock()

	if addon.page == nil {
		page, err := addon.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
		if err != nil {
			return nil, fmt.Errorf("failed to create page: %w", err)
		}
		addon.page = page
	}
	return addon.page, nil
}

func (addon *BrowserAddon) browserOpen(url string) string {
	ok, reason := addon.guard.Check("browser open: "+url, ActionRead)
	if !ok {
		return "guard: " + reason
	}

	page, err := addon.ensurePage()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	if err := page.Timeout(browserTimeout).Navigate(url); err != nil {
		return fmt.Sprintf("error navigating to %s: %v", url, err)
	}

	if err := page.Timeout(browserTimeout).WaitLoad(); err != nil {
		return fmt.Sprintf("warning: page load timeout for %s: %v", url, err)
	}

	info, err := page.Info()
	if err != nil {
		return fmt.Sprintf("opened: %s", url)
	}
	return fmt.Sprintf("opened: %s — %s", url, info.Title)
}

func (addon *BrowserAddon) browserScreenshot() string {
	page, err := addon.ensurePage()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	data, err := page.Timeout(browserTimeout).Screenshot(true, nil)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("screenshot (%d bytes): data:image/png;base64,%s", len(data), encoded)
}

func (addon *BrowserAddon) browserClick(selector string) string {
	page, err := addon.ensurePage()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("browser click: "+selector, ActionRead)
	if !ok {
		return "guard: " + reason
	}

	element, err := page.Timeout(browserTimeout).Element(selector)
	if err != nil {
		return fmt.Sprintf("error: element %q not found: %v", selector, err)
	}

	if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Sprintf("error clicking %q: %v", selector, err)
	}
	return fmt.Sprintf("clicked: %s", selector)
}

func (addon *BrowserAddon) browserType(selector, text string) string {
	page, err := addon.ensurePage()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	ok, reason := addon.guard.Check("browser type into: "+selector, ActionWrite)
	if !ok {
		return "guard: " + reason
	}

	element, err := page.Timeout(browserTimeout).Element(selector)
	if err != nil {
		return fmt.Sprintf("error: element %q not found: %v", selector, err)
	}

	if err := element.Input(text); err != nil {
		return fmt.Sprintf("error typing into %q: %v", selector, err)
	}
	return fmt.Sprintf("typed into %s: %q", selector, text)
}

func (addon *BrowserAddon) browserContent() string {
	page, err := addon.ensurePage()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	text, err := page.Timeout(browserTimeout).Eval(`() => document.body.innerText`)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	content := text.Value.String()
	if len(content) > 8000 {
		content = content[:8000] + "\n... (truncated)"
	}
	return content
}

func (addon *BrowserAddon) browserClose() string {
	if err := addon.closeBrowser(); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return "browser closed"
}

func (addon *BrowserAddon) closeBrowser() error {
	addon.mu.Lock()
	defer addon.mu.Unlock()

	if addon.page != nil {
		addon.page.Close()
		addon.page = nil
	}
	if addon.browser != nil {
		err := addon.browser.Close()
		addon.browser = nil
		return err
	}
	return nil
}
