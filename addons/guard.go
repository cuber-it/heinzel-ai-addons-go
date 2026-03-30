// ExecutionGuard — safety layer for shell/file operations.
// Three modes: paranoid, normal, expert.
// Blacklist always enforced, even in expert mode.

package addons

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// GuardMode controls how strict the guard is
type GuardMode string

const (
	ModeParanoid GuardMode = "paranoid" // ask for everything
	ModeNormal   GuardMode = "normal"   // ask for writes/deletes/exec
	ModeExpert   GuardMode = "expert"   // allow everything (except blacklist)
)

// ActionType classifies what an operation does
type ActionType int

const (
	ActionRead    ActionType = iota // reading files, listing dirs
	ActionWrite                     // writing/creating files
	ActionDelete                    // deleting files
	ActionExecute                   // running shell commands
)

// GuardEntry is a log entry for an executed action
type GuardEntry struct {
	Timestamp time.Time
	Action    string
	Type      ActionType
	Approved  bool
	Result    string
	Mode      GuardMode
}

// blacklist returns platform-specific dangerous commands — always blocked, no exceptions
var blacklist = platformBlacklist()

// ExecutionGuard validates and approves operations before execution
type ExecutionGuard struct {
	mu       sync.Mutex
	mode     GuardMode
	log      []GuardEntry
	approval func(action string) bool // callback: returns true if user approves
}

// NewExecutionGuard creates a guard with the given mode
func NewExecutionGuard(mode GuardMode, approvalFn func(action string) bool) *ExecutionGuard {
	if approvalFn == nil {
		approvalFn = func(action string) bool {
			fmt.Printf("[Guard] Approve? %s [y/N]: ", action)
			var input string
			fmt.Scanln(&input)
			return strings.ToLower(strings.TrimSpace(input)) == "y"
		}
	}
	return &ExecutionGuard{
		mode:     mode,
		approval: approvalFn,
	}
}

func (guard *ExecutionGuard) Mode() GuardMode {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	return guard.mode
}

func (guard *ExecutionGuard) SetMode(mode GuardMode) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.mode = mode
}

// Check validates an action and returns whether it is allowed.
// Blocks for user approval when required by the current mode.
func (guard *ExecutionGuard) Check(action string, actionType ActionType) (bool, string) {
	guard.mu.Lock()
	mode := guard.mode
	guard.mu.Unlock()

	lower := strings.ToLower(action)
	for _, pattern := range blacklist {
		if strings.Contains(lower, pattern) {
			reason := fmt.Sprintf("blocked: %q matches blacklist pattern %q — this command is permanently forbidden for safety. Try a safer alternative.", action, pattern)
			guard.logEntry(action, actionType, false, "BLOCKED (blacklist)", mode)
			log.Printf("[Guard] DENY blacklist action=%q pattern=%q mode=%s", action, pattern, mode)
			return false, reason
		}
	}

	if reason := guard.contextCheck(action, actionType); reason != "" {
		fullReason := "blocked: " + reason + " — consider using a path within your home directory or project workspace instead."
		guard.logEntry(action, actionType, false, "BLOCKED (context: "+reason+")", mode)
		log.Printf("[Guard] DENY context action=%q reason=%q mode=%s", action, reason, mode)
		return false, fullReason
	}

	needsApproval := false
	switch mode {
	case ModeParanoid:
		needsApproval = true
	case ModeNormal:
		needsApproval = actionType != ActionRead
	case ModeExpert:
		needsApproval = false
	}

	if needsApproval {
		typeLabel := actionTypeLabel(actionType)
		prompt := fmt.Sprintf("[%s] %s", typeLabel, action)
		if !guard.approval(prompt) {
			guard.logEntry(action, actionType, false, "DENIED by user", mode)
			log.Printf("[Guard] DENY user action=%q type=%s mode=%s", action, typeLabel, mode)
			return false, fmt.Sprintf("denied by user — the %s operation was not approved. You can change the execution mode to 'expert' if you want automatic approval.", typeLabel)
		}
		log.Printf("[Guard] ALLOW (user-approved) action=%q type=%s mode=%s", action, typeLabel, mode)
	} else {
		log.Printf("[Guard] ALLOW action=%q type=%s mode=%s", action, actionTypeLabel(actionType), mode)
	}

	guard.logEntry(action, actionType, true, "OK", mode)
	return true, ""
}

func (guard *ExecutionGuard) Log() []GuardEntry {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	result := make([]GuardEntry, len(guard.log))
	copy(result, guard.log)
	return result
}

func (guard *ExecutionGuard) contextCheck(action string, actionType ActionType) string {
	lower := strings.ToLower(action)

	systemPaths := []string{"/etc/", "/usr/", "/bin/", "/sbin/", "/boot/", "/proc/", "/sys/"}
	if actionType == ActionWrite || actionType == ActionDelete {
		for _, sysPath := range systemPaths {
			if strings.Contains(lower, sysPath) {
				return fmt.Sprintf("operation targets system path %q", sysPath)
			}
		}
	}

	if actionType == ActionExecute {
		dangerousPipes := []string{"| sh", "| bash", "| zsh", "curl | ", "wget | "}
		for _, pipe := range dangerousPipes {
			if strings.Contains(lower, pipe) {
				return fmt.Sprintf("suspicious pipe pattern: %q", pipe)
			}
		}
	}

	return ""
}

func (guard *ExecutionGuard) logEntry(action string, actionType ActionType, approved bool, result string, mode GuardMode) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.log = append(guard.log, GuardEntry{
		Timestamp: time.Now(),
		Action:    action,
		Type:      actionType,
		Approved:  approved,
		Result:    result,
		Mode:      mode,
	})
}

func actionTypeLabel(actionType ActionType) string {
	switch actionType {
	case ActionRead:
		return "READ"
	case ActionWrite:
		return "WRITE"
	case ActionDelete:
		return "DELETE"
	case ActionExecute:
		return "EXEC"
	default:
		return "UNKNOWN"
	}
}
