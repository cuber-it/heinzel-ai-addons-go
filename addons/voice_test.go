package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewVoiceAddon(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	if addon == nil {
		test.Fatal("NewVoiceAddon returned nil")
	}
	if addon.Name() != "voice" {
		test.Errorf("expected name 'voice', got %q", addon.Name())
	}
	if addon.Type() != core.AddonTool {
		test.Errorf("expected type AddonTool, got %v", addon.Type())
	}
}

func TestVoiceAddonDefaultMode(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	if addon.mode() != VoiceModeOff {
		test.Errorf("expected default mode 'off', got %q", addon.mode())
	}
}

func TestVoiceAddonDefaultAPIBase(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	if addon.config.APIBase != "https://api.openai.com/v1" {
		test.Errorf("expected default API base, got %q", addon.config.APIBase)
	}
}

func TestVoiceAddonExplicitMode(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeCloud, APIKey: "test-key"})
	if addon.mode() != VoiceModeCloud {
		test.Errorf("expected cloud mode, got %q", addon.mode())
	}
}

func TestVoiceAddonStatusShowsModeAndBackends(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeOff})
	status := addon.statusText()

	if !strings.Contains(status, "Voice: off") {
		test.Errorf("expected 'Voice: off' in status, got: %s", status)
	}
	if !strings.Contains(status, "available:") {
		test.Errorf("expected 'available:' in status, got: %s", status)
	}
}

func TestVoiceAddonStatusWithAPIKey(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeCloud, APIKey: "sk-test"})
	status := addon.statusText()

	if !strings.Contains(status, "Voice: cloud") {
		test.Errorf("expected 'Voice: cloud' in status, got: %s", status)
	}
	if !strings.Contains(status, "cloud") {
		test.Errorf("expected 'cloud' backend available, got: %s", status)
	}
}

func TestVoiceAddonHandleCommandStatus(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	ctx := testContext()

	result := addon.HandleCommand("voice", "status", ctx)
	if !strings.Contains(result, "Voice:") {
		test.Errorf("expected 'Voice:' in command result, got: %s", result)
	}
}

func TestVoiceAddonHandleCommandOff(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeCloud, APIKey: "test"})
	ctx := testContext()

	result := addon.HandleCommand("voice", "off", ctx)
	if !strings.Contains(result, "disabled") {
		test.Errorf("expected 'disabled' in result, got: %s", result)
	}
	if addon.mode() != VoiceModeOff {
		test.Errorf("expected mode off after /voice off, got %q", addon.mode())
	}
}

func TestVoiceAddonHandleCommandOnNoBackend(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeOff})
	ctx := testContext()

	result := addon.HandleCommand("voice", "on", ctx)
	if !strings.Contains(result, "no backend") {
		test.Errorf("expected 'no backend' error, got: %s", result)
	}
}

func TestVoiceAddonHandleCommandOnWithAPIKey(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeOff, APIKey: "sk-test"})
	ctx := testContext()

	result := addon.HandleCommand("voice", "on", ctx)
	if !strings.Contains(result, "enabled") {
		test.Errorf("expected 'enabled' in result, got: %s", result)
	}
	if addon.mode() != VoiceModeCloud {
		test.Errorf("expected cloud mode after enabling with API key, got %q", addon.mode())
	}
}

func TestVoiceAddonHandleCommandEmpty(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	ctx := testContext()

	result := addon.HandleCommand("voice", "", ctx)
	// Empty args should show status
	if !strings.Contains(result, "Voice:") {
		test.Errorf("expected status output for empty args, got: %s", result)
	}
}

func TestVoiceAddonHandleOnOutputWhenOff(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{Mode: VoiceModeOff})
	ctx := testContext()
	ctx.Output = "Test output"

	result := addon.Handle(core.OnOutput, ctx)
	// Should not halt and not store audio when off
	if result.Halt {
		test.Error("expected no halt when voice is off")
	}
	_, hasAudio := ctx.Get("voice:audio")
	if hasAudio {
		test.Error("expected no audio when voice is off")
	}
}

func TestVoiceAddonHooks(test *testing.T) {
	addon := NewVoiceAddon(VoiceConfig{})
	hooks := addon.Hooks()
	if len(hooks) != 1 {
		test.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0] != core.OnOutput {
		test.Errorf("expected OnOutput hook, got %v", hooks[0])
	}
}
