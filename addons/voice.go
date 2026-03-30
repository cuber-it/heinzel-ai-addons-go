// Voice Addon — STT (Whisper) and TTS for the Heinzel loop.
// Cloud mode uses OpenAI API, local mode uses whisper.cpp CLI + piper CLI via subprocess.

package addons

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// VoiceMode controls which backend is used
type VoiceMode string

const (
	VoiceModeCloud VoiceMode = "cloud"
	VoiceModeLocal VoiceMode = "local"
	VoiceModeOff   VoiceMode = "off"
)

// VoiceConfig holds the voice addon configuration
type VoiceConfig struct {
	Mode       VoiceMode // "cloud", "local", "off"
	APIKey     string    // OpenAI API key (cloud mode)
	APIBase    string    // API base URL (default: https://api.openai.com/v1)
	PiperModel string   // Piper voice model path (local mode)
}

// VoiceAddon provides speech-to-text and text-to-speech
type VoiceAddon struct {
	core.BaseAddon
	mu     sync.RWMutex
	config VoiceConfig
	client *http.Client

	// local binary paths (resolved at Start)
	whisperBin string // path to whisper-cli or whisper.cpp
	piperBin   string // path to piper
}

func NewVoiceAddon(config VoiceConfig) *VoiceAddon {
	if config.APIBase == "" {
		config.APIBase = "https://api.openai.com/v1"
	}
	if config.Mode == "" {
		config.Mode = VoiceModeOff
	}
	return &VoiceAddon{
		config: config,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (addon *VoiceAddon) Name() string        { return "voice" }
func (addon *VoiceAddon) Type() core.AddonType { return core.AddonTool }

func (addon *VoiceAddon) Start() error {
	addon.whisperBin = findBinary("whisper-cli", "whisper.cpp", "whisper")
	addon.piperBin = findBinary("piper")

	if addon.whisperBin != "" {
		log.Printf("[voice] whisper binary found: %s", addon.whisperBin)
	}
	if addon.piperBin != "" {
		log.Printf("[voice] piper binary found: %s", addon.piperBin)
	}

	addon.mu.Lock()
	defer addon.mu.Unlock()

	if addon.config.Mode == VoiceModeOff {
		if addon.config.APIKey != "" {
			addon.config.Mode = VoiceModeCloud
			log.Printf("[voice] mode: cloud (OpenAI API key set)")
		} else if addon.whisperBin != "" && addon.piperBin != "" {
			addon.config.Mode = VoiceModeLocal
			log.Printf("[voice] mode: local (whisper + piper found)")
		} else {
			log.Printf("[voice] mode: off (no API key, local binaries: whisper=%v piper=%v)",
				addon.whisperBin != "", addon.piperBin != "")
		}
	}

	return nil
}

func (addon *VoiceAddon) Stop() error { return nil }

func (addon *VoiceAddon) Hooks() []core.HookPoint {
	return []core.HookPoint{core.OnOutput}
}

func (addon *VoiceAddon) Commands() []core.Command {
	return []core.Command{
		{Name: "voice", Description: "voice control", Usage: "voice on|off|status"},
	}
}

func (addon *VoiceAddon) HandleCommand(cmd, args string, ctx *core.Context) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return addon.statusText()
	}

	switch parts[0] {
	case "on":
		addon.mu.Lock()
		if addon.config.Mode == VoiceModeOff {
			if addon.config.APIKey != "" {
				addon.config.Mode = VoiceModeCloud
			} else if addon.whisperBin != "" && addon.piperBin != "" {
				addon.config.Mode = VoiceModeLocal
			} else {
				addon.mu.Unlock()
				return "Voice: no backend available (set OPENAI_API_KEY or install whisper.cpp + piper)"
			}
		}
		addon.mu.Unlock()
		return "Voice enabled (" + string(addon.mode()) + ")"
	case "off":
		addon.mu.Lock()
		addon.config.Mode = VoiceModeOff
		addon.mu.Unlock()
		return "Voice disabled"
	case "status":
		return addon.statusText()
	default:
		return "Usage: /voice on|off|status"
	}
}

func (addon *VoiceAddon) Handle(hook core.HookPoint, ctx *core.Context) core.Result {
	if hook != core.OnOutput {
		return core.Result{}
	}

	mode := addon.mode()
	if mode == VoiceModeOff || ctx.Output == "" {
		return core.Result{}
	}

	audioBytes, err := addon.Speak(ctx.Output, "")
	if err != nil {
		return core.Result{}
	}

	ctx.Set("voice:audio", audioBytes)
	return core.Result{}
}

func (addon *VoiceAddon) Transcribe(audioData []byte, language string) (string, error) {
	mode := addon.mode()
	switch mode {
	case VoiceModeCloud:
		return addon.transcribeCloud(audioData, language)
	case VoiceModeLocal:
		return addon.transcribeLocal(audioData, language)
	default:
		return "", fmt.Errorf("voice is off")
	}
}

func (addon *VoiceAddon) Speak(text string, voice string) ([]byte, error) {
	mode := addon.mode()
	switch mode {
	case VoiceModeCloud:
		return addon.speakCloud(text, voice)
	case VoiceModeLocal:
		return addon.speakLocal(text)
	default:
		return nil, fmt.Errorf("voice is off")
	}
}

// --- cloud implementation ---

func (addon *VoiceAddon) transcribeCloud(audioData []byte, language string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "audio.webm")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(audioData)); err != nil {
		return "", fmt.Errorf("copy audio: %w", err)
	}
	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	if language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return "", fmt.Errorf("write language field: %w", err)
		}
	}
	writer.Close()

	addon.mu.RLock()
	apiBase := addon.config.APIBase
	apiKey := addon.config.APIKey
	addon.mu.RUnlock()

	req, err := http.NewRequest("POST", apiBase+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := addon.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("whisper %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return result.Text, nil
}

func (addon *VoiceAddon) speakCloud(text string, voice string) ([]byte, error) {
	if voice == "" {
		voice = "nova"
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": "tts-1",
		"input": text,
		"voice": voice,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	addon.mu.RLock()
	apiBase := addon.config.APIBase
	apiKey := addon.config.APIKey
	addon.mu.RUnlock()

	req, err := http.NewRequest("POST", apiBase+"/audio/speech", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := addon.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// --- local implementation (whisper.cpp + piper subprocess) ---

func (addon *VoiceAddon) transcribeLocal(audioData []byte, language string) (string, error) {
	if addon.whisperBin == "" {
		return "", fmt.Errorf("whisper.cpp not found on PATH — install from https://github.com/ggerganov/whisper.cpp")
	}

	tmpDir, err := os.MkdirTemp("", "heinzel-voice-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	audioPath := filepath.Join(tmpDir, "audio.wav")
	if err := os.WriteFile(audioPath, audioData, 0600); err != nil {
		return "", fmt.Errorf("write audio temp: %w", err)
	}

	args := []string{
		"--model", "small",
		"--output-txt",
		"--output-file", filepath.Join(tmpDir, "audio"),
	}
	if language != "" {
		args = append(args, "--language", language)
	} else {
		args = append(args, "--language", "auto")
	}
	args = append(args, audioPath)

	cmd := exec.Command(addon.whisperBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper failed: %w — %s", err, stderr.String())
	}

	txtPath := filepath.Join(tmpDir, "audio.txt")
	result, err := os.ReadFile(txtPath)
	if err != nil {
		return "", fmt.Errorf("read whisper output: %w", err)
	}

	return strings.TrimSpace(string(result)), nil
}

func (addon *VoiceAddon) speakLocal(text string) ([]byte, error) {
	if addon.piperBin == "" {
		return nil, fmt.Errorf("piper not found on PATH — install from https://github.com/rhasspy/piper")
	}

	args := []string{"--output-raw"}

	addon.mu.RLock()
	piperModel := addon.config.PiperModel
	addon.mu.RUnlock()

	if piperModel != "" {
		args = append(args, "--model", piperModel)
	}

	cmd := exec.Command(addon.piperBin, args...)
	cmd.Stdin = strings.NewReader(text)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("piper failed: %w — %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

func findBinary(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// --- helpers ---

func (addon *VoiceAddon) mode() VoiceMode {
	addon.mu.RLock()
	defer addon.mu.RUnlock()
	return addon.config.Mode
}

func (addon *VoiceAddon) statusText() string {
	mode := addon.mode()

	var backends []string
	addon.mu.RLock()
	hasAPIKey := addon.config.APIKey != ""
	addon.mu.RUnlock()

	if hasAPIKey {
		backends = append(backends, "cloud")
	}
	if addon.whisperBin != "" && addon.piperBin != "" {
		backends = append(backends, "local")
	} else if addon.whisperBin != "" {
		backends = append(backends, "local(whisper only, piper missing)")
	} else if addon.piperBin != "" {
		backends = append(backends, "local(piper only, whisper missing)")
	}

	available := "none"
	if len(backends) > 0 {
		available = strings.Join(backends, ", ")
	}

	switch mode {
	case VoiceModeCloud:
		return fmt.Sprintf("Voice: cloud (OpenAI Whisper + TTS) | available: %s", available)
	case VoiceModeLocal:
		return fmt.Sprintf("Voice: local (whisper.cpp + piper) | available: %s", available)
	default:
		return fmt.Sprintf("Voice: off | available: %s", available)
	}
}
