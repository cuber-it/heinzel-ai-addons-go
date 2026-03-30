// WebSearchAddon — web search via SearXNG and URL content fetching.

package addons

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"

	"github.com/jaytaylor/html2text"
)

const (
	maxSearchResults = 8
	searchPreviewLength = 200
	maxContentLength = 8000
)

// WebSearchAddon provides built-in web search and URL fetching
// Not an MCP tool — a core capability like file upload
type WebSearchAddon struct {
	core.BaseAddon
	client      *http.Client
	searchKey   string // for Brave/Google/Bing
	engine      string // searxng, brave, ddg, none
	searxngURL  string
}

func NewWebSearchAddon() *WebSearchAddon {
	return &WebSearchAddon{
		client: &http.Client{Timeout: 15 * time.Second},
		engine: "none",
	}
}

func NewWebSearchAddonWithSearXNG(searxngURL string) *WebSearchAddon {
	return &WebSearchAddon{
		client:     &http.Client{Timeout: 15 * time.Second},
		engine:     "searxng",
		searxngURL: strings.TrimRight(searxngURL, "/"),
	}
}

func (addon *WebSearchAddon) Name() string           { return "websearch" }
func (addon *WebSearchAddon) Type() core.AddonType { return core.AddonTool }
func (addon *WebSearchAddon) Start() error {
	addon.searchKey = os.Getenv("SERPAPI_KEY") // optional, for better results
	return nil
}
func (addon *WebSearchAddon) Stop() error { return nil }

func (addon *WebSearchAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{
		core.OnInput,       // detect URLs and search intent
		core.OnLLMResponse, // fallback search when LLM says "I don't know"
	}
}

func (addon *WebSearchAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "web", Description: "web search or fetch URL", Usage: "/web search <query>\n  /web fetch <url>"},
	}
}

func (addon *WebSearchAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		return "Usage: /web search <query> | /web fetch <url>"
	}
	switch parts[0] {
	case "search":
		return addon.search(parts[1])
	case "fetch":
		return addon.fetch(parts[1])
	}
	return "Usage: /web search <query> | /web fetch <url>"
}

func (addon *WebSearchAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	// Fallback: LLM said "I don't know" — search and re-run (ONCE only)
	if hook == core.OnLLMResponse {
		if query, ok := ctx.Get(KeyNeedsWebSearch); ok {
			queryStr, ok := query.(string)
			if ok && queryStr != "" {
			if _, already := ctx.Get(KeyWebFallbackDone); already {
					ctx.Set(KeyNeedsWebSearch, nil)
					return core.Result{}
				}
				thinking := core.GetThinking(ctx)
				if thinking != nil {
					thinking.AddStep("tool", "auto web search: "+queryStr, "websearch")
				}
				result := addon.search(queryStr)
				if result != "" && !strings.HasPrefix(result, "Keine") {
					ctx.Prompts.Add(core.LayerTurn, "websearch:fallback",
						"Websuche ergab:\n"+result, 80)
					ctx.Set(KeyNeedsRerun, true)
					ctx.Output = ""
				}
				ctx.Set(KeyNeedsWebSearch, nil)
				ctx.Set(KeyWebFallbackDone, true)
			}
		}
		return core.Result{}
	}

	input := strings.ToLower(ctx.Input)

	searchTriggers := []string{"suche ", "search ", "recherche ", "finde ", "was ist ",
		"wer ist ", "google ", "look up ", "im web ", "im internet ",
		"findest du ", "was weißt du über ", "was weisst du über "}
	for _, trigger := range searchTriggers {
		if strings.Contains(input, trigger) {
			query := ctx.Input
			result := addon.search(query)
			ctx.Prompts.Add(core.LayerTurn, "websearch", "Suchergebnisse:\n"+result, 80)
			thinking := core.GetThinking(ctx)
			if thinking != nil {
				thinking.AddStep("tool", "web search: "+query, "websearch")
			}
			break
		}
	}

	words := strings.Fields(ctx.Input)
	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") ||
			strings.HasPrefix(word, "www.") {
			fetchURL := word
			if strings.HasPrefix(fetchURL, "www.") {
				fetchURL = "https://" + fetchURL
			}
			result := addon.fetch(fetchURL)
			ctx.Prompts.Add(core.LayerTurn, "webfetch:"+word, "Inhalt von "+word+":\n"+result, 75)
			thinking := core.GetThinking(ctx)
			if thinking != nil {
				thinking.AddStep("tool", "fetched: "+word, "websearch")
			}
		}
	}

	return core.Result{}
}

func (addon *WebSearchAddon) search(query string) string {
	switch addon.engine {
	case "searxng":
		return addon.searxngSearch(query)
	case "brave":
		return addon.braveSearch(query)
	case "ddg":
		return addon.ddgSearch(query)
	default:
		return "Keine Suchmaschine konfiguriert. Nutze /web fetch <url> für direkte URL-Abrufe."
	}
}

func (addon *WebSearchAddon) searxngSearch(query string) string {
	reqURL := fmt.Sprintf("%s/search?q=%s&format=json&language=de", addon.searxngURL, url.QueryEscape(query))
	resp, err := addon.client.Get(reqURL)
	if err != nil {
		return fmt.Sprintf("SearXNG Fehler: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Sprintf("SearXNG Parse-Fehler: %v", err)
	}

	if len(result.Results) == 0 {
		return "Keine Ergebnisse für: " + query
	}

	var lines []string
	limit := maxSearchResults
	if len(result.Results) < limit {
		limit = len(result.Results)
	}
	for _, res := range result.Results[:limit] {
		line := fmt.Sprintf("- %s\n  %s", res.Title, res.URL)
		if res.Content != "" {
			content := res.Content
			if len(content) > searchPreviewLength {
				content = content[:searchPreviewLength] + "..."
			}
			line += fmt.Sprintf("\n  %s", content)
		}
		lines = append(lines, line)
	}
	return fmt.Sprintf("Suchergebnisse (%d Treffer):\n%s", len(result.Results), strings.Join(lines, "\n"))
}

func (addon *WebSearchAddon) braveSearch(query string) string {
	reqURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=5", url.QueryEscape(query))
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Subscription-Token", addon.searchKey)
	req.Header.Set("Accept", "application/json")

	resp, err := addon.client.Do(req)
	if err != nil {
		return fmt.Sprintf("Brave Search Fehler: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Sprintf("Brave Search Parse-Fehler: %v", err)
	}

	var lines []string
	for _, res := range result.Web.Results {
		lines = append(lines, fmt.Sprintf("- %s\n  %s\n  %s", res.Title, res.Description, res.URL))
	}
	if len(lines) == 0 {
		return "Keine Ergebnisse."
	}
	return strings.Join(lines, "\n")
}

func (addon *WebSearchAddon) ddgSearch(query string) string {
	reqURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1", url.QueryEscape(query))
	resp, err := addon.client.Get(reqURL)
	if err != nil {
		return fmt.Sprintf("Suche fehlgeschlagen: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Abstract     string `json:"Abstract"`
		AbstractURL  string `json:"AbstractURL"`
		AbstractText string `json:"AbstractText"`
		Heading      string `json:"Heading"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Sprintf("DuckDuckGo Parse-Fehler: %v", err)
	}

	var lines []string
	if result.Heading != "" {
		lines = append(lines, result.Heading)
	}
	if result.AbstractText != "" {
		lines = append(lines, result.AbstractText)
		if result.AbstractURL != "" {
			lines = append(lines, "Quelle: "+result.AbstractURL)
		}
	}
	for idx, topic := range result.RelatedTopics {
		if idx >= 5 {
			break
		}
		if topic.Text != "" {
			lines = append(lines, fmt.Sprintf("- %s", topic.Text))
		}
	}

	if len(lines) == 0 {
		return "Keine Ergebnisse für: " + query
	}
	return strings.Join(lines, "\n")
}

func (addon *WebSearchAddon) fetch(rawURL string) string {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return fmt.Sprintf("Ungültige URL: %v", err)
	}
	req.Header.Set("User-Agent", "Neo-Heinzel/0.1")

	resp, err := addon.client.Do(req)
	if err != nil {
		return fmt.Sprintf("Abruf fehlgeschlagen: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	content, err := html2text.FromString(string(body), html2text.Options{
		PrettyTables: true,
		OmitLinks:    false,
	})
	if err != nil {
		content = string(body)
	}

	if len(content) > maxContentLength {
		content = content[:maxContentLength] + "\n... (gekürzt)"
	}

	return content
}
