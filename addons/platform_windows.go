//go:build windows

package addons

import (
	"os"
	"os/exec"
)

func defaultShell() string   { return "powershell.exe" }
func shellFlag() string      { return "-Command" }
func openBrowser(url string) { exec.Command("cmd", "/c", "start", url).Start() }

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "C:\\Temp"
	}
	return home
}
