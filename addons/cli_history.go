// CLI history — load and save readline history.

package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxHistoryEntries = 500

func (cli *CLIBridge) loadHistory() {
	data, err := os.ReadFile(cli.historyFile)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cli.history = append(cli.history, line)
		}
	}
}

func (cli *CLIBridge) saveHistory() {
	os.MkdirAll(filepath.Dir(cli.historyFile), 0755)
	entries := cli.history
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}
	os.WriteFile(cli.historyFile, []byte(strings.Join(entries, "\n")+"\n"), 0644)
	fmt.Printf("  %sHistory saved (%d entries)%s\n", colorGreen, len(entries), colorReset)
}
