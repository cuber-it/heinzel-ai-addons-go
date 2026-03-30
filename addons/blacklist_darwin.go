//go:build darwin

package addons

func platformBlacklist() []string {
	return []string{
		"rm -rf /",
		"rm -rf ~",
		"sudo rm",
		"mkfs",
		"dd if=",
		"> /dev/sd",
	}
}
