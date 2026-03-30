package addons

import (
	"strings"
	"testing"
)

func TestSkillStructCreation(test *testing.T) {
	skill := &Skill{
		Name:        "code-review",
		Description: "Review Go code",
		Output: SkillOutput{
			Format:   "markdown",
			Language: "go",
			Sections: []string{"Summary", "Issues", "Suggestions"},
			MaxLines: 100,
		},
		Quality:   []string{"no false positives", "actionable suggestions"},
		Review:    []string{"all issues addressed", "no new bugs"},
		Templates: map[string]string{"default": "Review this code: {{.Code}}"},
	}

	if skill.Name != "code-review" {
		test.Errorf("expected name 'code-review', got %q", skill.Name)
	}
	if skill.Output.Format != "markdown" {
		test.Errorf("expected format 'markdown', got %q", skill.Output.Format)
	}
	if len(skill.Output.Sections) != 3 {
		test.Errorf("expected 3 sections, got %d", len(skill.Output.Sections))
	}
	if len(skill.Quality) != 2 {
		test.Errorf("expected 2 quality criteria, got %d", len(skill.Quality))
	}
}

func TestSkillRegistryRegisterAndGet(test *testing.T) {
	registry := NewSkillRegistry()

	skill := &Skill{Name: "code-review", Description: "Review code"}
	registry.Register(skill)

	got := registry.Get("code-review")
	if got == nil {
		test.Fatal("expected skill, got nil")
	}
	if got.Name != "code-review" {
		test.Errorf("expected 'code-review', got %q", got.Name)
	}
}

func TestSkillRegistryGetNonexistent(test *testing.T) {
	registry := NewSkillRegistry()

	got := registry.Get("nonexistent")
	if got != nil {
		test.Errorf("expected nil for nonexistent skill, got %v", got)
	}
}

func TestSkillRegistryNames(test *testing.T) {
	registry := NewSkillRegistry()
	registry.Register(&Skill{Name: "alpha"})
	registry.Register(&Skill{Name: "beta"})

	names := registry.Names()
	if len(names) != 2 {
		test.Fatalf("expected 2 names, got %d", len(names))
	}

	nameSet := map[string]bool{}
	for _, name := range names {
		nameSet[name] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		test.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestSkillRegistryAll(test *testing.T) {
	registry := NewSkillRegistry()
	registry.Register(&Skill{Name: "alpha"})
	registry.Register(&Skill{Name: "beta"})

	all := registry.All()
	if len(all) != 2 {
		test.Errorf("expected 2 skills, got %d", len(all))
	}
}

func TestSkillRegistryOverwrite(test *testing.T) {
	registry := NewSkillRegistry()
	registry.Register(&Skill{Name: "alpha", Description: "v1"})
	registry.Register(&Skill{Name: "alpha", Description: "v2"})

	got := registry.Get("alpha")
	if got.Description != "v2" {
		test.Errorf("expected overwritten description 'v2', got %q", got.Description)
	}
	if len(registry.Names()) != 1 {
		test.Errorf("expected 1 skill after overwrite, got %d", len(registry.Names()))
	}
}

func TestFormatAsPrompt(test *testing.T) {
	skill := &Skill{
		Name:        "code-review",
		Description: "Review Go code for quality",
		Output: SkillOutput{
			Format:   "markdown",
			Language: "go",
			Sections: []string{"Summary", "Issues"},
		},
		Quality: []string{"no false positives"},
		Review:  []string{"all issues addressed"},
	}

	prompt := skill.FormatAsPrompt()

	checks := []string{
		"=== Skill: code-review ===",
		"Review Go code for quality",
		"Output-Format: markdown",
		"Sprache: go",
		"Erforderliche Abschnitte:",
		"Summary",
		"Issues",
		"Qualitätskriterien:",
		"no false positives",
		"Review-Checkliste",
		"[ ] all issues addressed",
	}

	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			test.Errorf("expected %q in prompt, got:\n%s", check, prompt)
		}
	}
}

func TestFormatAsPromptMinimal(test *testing.T) {
	skill := &Skill{Name: "minimal"}
	prompt := skill.FormatAsPrompt()

	if !strings.Contains(prompt, "=== Skill: minimal ===") {
		test.Errorf("expected skill header, got %q", prompt)
	}
}

func TestFormatReviewPrompt(test *testing.T) {
	skill := &Skill{
		Name:    "review",
		Quality: []string{"accurate", "complete"},
		Review:  []string{"no regressions"},
	}

	prompt := skill.FormatReviewPrompt("some output text")

	if !strings.Contains(prompt, "Qualitätskriterien:") {
		test.Error("expected quality criteria section")
	}
	if !strings.Contains(prompt, "accurate") {
		test.Error("expected quality criterion in output")
	}
	if !strings.Contains(prompt, "Review-Checkliste:") {
		test.Error("expected review checklist section")
	}
	if !strings.Contains(prompt, "some output text") {
		test.Error("expected output included in review prompt")
	}
}
