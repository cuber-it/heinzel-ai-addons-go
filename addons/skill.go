// Skill definitions and registry.
package addons

// Skill defines what a task must produce — format, quality, review criteria
type Skill struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Output      SkillOutput       `yaml:"output" json:"output"`
	Quality     []string          `yaml:"quality" json:"quality"`
	Review      []string          `yaml:"review" json:"review"`
	Templates   map[string]string `yaml:"templates" json:"templates"` // named prompt templates
}

// SkillOutput defines the expected deliverable
type SkillOutput struct {
	Format   string   `yaml:"format" json:"format"`     // markdown, code, json, yaml
	Language string   `yaml:"language" json:"language"`  // go, python, etc.
	Sections []string `yaml:"sections" json:"sections"`  // required sections
	MaxLines int      `yaml:"max_lines" json:"max_lines"`
}

// SkillRegistry holds all available skills
type SkillRegistry struct {
	skills map[string]*Skill
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{skills: make(map[string]*Skill)}
}

func (reg *SkillRegistry) Register(skill *Skill) {
	reg.skills[skill.Name] = skill
}

func (reg *SkillRegistry) Get(name string) *Skill {
	return reg.skills[name]
}

func (reg *SkillRegistry) All() []*Skill {
	var result []*Skill
	for _, skill := range reg.skills {
		result = append(result, skill)
	}
	return result
}

func (reg *SkillRegistry) Names() []string {
	var names []string
	for name := range reg.skills {
		names = append(names, name)
	}
	return names
}

// FormatAsPrompt converts a skill into LLM instructions
func (skill *Skill) FormatAsPrompt() string {
	var parts []string

	parts = append(parts, "=== Skill: "+skill.Name+" ===")

	if skill.Description != "" {
		parts = append(parts, skill.Description)
	}

	if skill.Output.Format != "" {
		parts = append(parts, "\nOutput-Format: "+skill.Output.Format)
	}
	if skill.Output.Language != "" {
		parts = append(parts, "Sprache: "+skill.Output.Language)
	}
	if len(skill.Output.Sections) > 0 {
		parts = append(parts, "\nErforderliche Abschnitte:")
		for _, section := range skill.Output.Sections {
			parts = append(parts, "  - "+section)
		}
	}

	if len(skill.Quality) > 0 {
		parts = append(parts, "\nQualitätskriterien:")
		for _, criterion := range skill.Quality {
			parts = append(parts, "  - "+criterion)
		}
	}

	if len(skill.Review) > 0 {
		parts = append(parts, "\nReview-Checkliste (prüfe am Ende):")
		for _, check := range skill.Review {
			parts = append(parts, "  - [ ] "+check)
		}
	}

	result := ""
	for _, part := range parts {
		result += part + "\n"
	}
	return result
}

// FormatReviewPrompt creates a prompt for quality review
func (skill *Skill) FormatReviewPrompt(output string) string {
	var parts []string
	parts = append(parts, "Prüfe das folgende Ergebnis gegen die Qualitätskriterien.")
	parts = append(parts, "Antworte mit PASS oder FAIL + Begründung für jeden Punkt.\n")

	if len(skill.Quality) > 0 {
		parts = append(parts, "Qualitätskriterien:")
		for _, criterion := range skill.Quality {
			parts = append(parts, "  - "+criterion)
		}
	}
	if len(skill.Review) > 0 {
		parts = append(parts, "\nReview-Checkliste:")
		for _, check := range skill.Review {
			parts = append(parts, "  - "+check)
		}
	}

	parts = append(parts, "\n--- Ergebnis ---\n")
	parts = append(parts, output)

	result := ""
	for _, part := range parts {
		result += part + "\n"
	}
	return result
}
