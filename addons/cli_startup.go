// CLI startup — RC-files, startup docs, project detection, bash execution.

package addons

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func (cli *CLIBridge) SetStartupDocs(docs []core.StartupDoc) {
	cli.startupDocs = docs
}

// loadRC loads and executes rc files in hierarchy:
// /etc/neo-heinzel/rc → ~/.neo-heinzel/rc → ./.heinzelrc
func (cli *CLIBridge) loadRC(loop *core.Loop, ctx *core.Context) {
	home, _ := os.UserHomeDir()
	rcPaths := []string{
		"/etc/neo-heinzel/rc",
		filepath.Join(home, ".neo-heinzel", "rc"),
		".heinzelrc",
	}

	for _, path := range rcPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		cli.rcFiles = append(cli.rcFiles, path)
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// Execute as command or input
			if strings.HasPrefix(line, "/") {
				cli.handleCommand(line, loop, ctx)
			} else {
				loop.Run(ctx, line)
			}
		}
	}

	if len(cli.rcFiles) > 0 {
		fmt.Printf("%sLoaded: %s%s\n", colorGray, strings.Join(cli.rcFiles, ", "), colorReset)
	}
}

func (cli *CLIBridge) detectProject(ctx *core.Context) {
	var detected []string

	checks := map[string]string{
		"go.mod":         "Go",
		"pyproject.toml": "Python",
		"package.json":   "Node.js",
		"Cargo.toml":     "Rust",
		"pom.xml":        "Java",
		"Gemfile":        "Ruby",
		"composer.json":  "PHP",
		"Makefile":       "Make",
		"Dockerfile":     "Docker",
		".git":           "Git",
	}

	for file, lang := range checks {
		if _, err := os.Stat(file); err == nil {
			detected = append(detected, lang)
		}
	}

	if len(detected) > 0 {
		ctx.Prompts.Add(core.LayerSession, "project",
			fmt.Sprintf("Projekt erkannt: %s\nArbeitsverzeichnis: %s",
				strings.Join(detected, ", "), mustGetwd()), 50)
		fmt.Printf("%sProjekt: %s%s\n", colorGray, strings.Join(detected, ", "), colorReset)
	}

	if data, err := os.ReadFile(".riker.md"); err == nil {
		ctx.Prompts.Add(core.LayerSession, "riker_md",
			"Projekt-Instruktionen:\n"+string(data), 80)
		fmt.Printf("%sGeladen: .riker.md%s\n", colorGray, colorReset)
	}
}

func (cli *CLIBridge) loadStartupDocs(docs []core.StartupDoc, ctx *core.Context) {
	for _, doc := range docs {
		path := doc.Path
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if !doc.Optional {
				fmt.Printf("  %sWarning: %s nicht gefunden%s\n", colorYellow, doc.Path, colorReset)
			}
			continue
		}

		layer := core.LayerSession
		switch doc.Layer {
		case "system":
			layer = core.LayerSystem
		case "user":
			layer = core.LayerUser
		case "turn":
			layer = core.LayerTurn
		}

		priority := doc.Priority
		if priority == 0 {
			priority = 50
		}

		ctx.Prompts.Add(layer, "doc:"+doc.Path, string(data), priority)
		fmt.Printf("%sGeladen: %s%s\n", colorGray, doc.Path, colorReset)
	}
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func (cli *CLIBridge) executeBash(cmd string, ctx *core.Context) {
	fmt.Printf("  %s$ %s%s\n", colorGray, cmd, colorReset)
	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	output := strings.TrimRight(string(out), "\n")
	if output != "" {
		fmt.Println(output)
	}
	if err != nil {
		fmt.Printf("  %sExit: %v%s\n", colorRed, err, colorReset)
	}
	if output != "" {
		ctx.Prompts.Add(core.LayerTurn, "bash", fmt.Sprintf("Shell-Ausgabe von `%s`:\n```\n%s\n```", cmd, output), 70)
	}
}
