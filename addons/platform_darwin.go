//go:build darwin

package addons

import (
	"os"
	"os/exec"
)

func defaultShell() string   { return "/bin/zsh" }
func shellFlag() string      { return "-c" }
func openBrowser(url string) { exec.Command("open", url).Start() }

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return home
}
