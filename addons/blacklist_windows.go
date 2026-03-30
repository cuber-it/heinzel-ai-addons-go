//go:build windows

package addons

func platformBlacklist() []string {
	return []string{
		"remove-item -recurse -force c:\\",
		"remove-item -recurse -force $home",
		"format ",
		"del /s /q c:\\",
		"rd /s /q c:\\",
		"stop-computer",
		"restart-computer -force",
	}
}
