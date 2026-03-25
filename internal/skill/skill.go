package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tiny-oc/toc/internal/config"
	"gopkg.in/yaml.v3"
)

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SkillMeta represents the YAML frontmatter parsed from SKILL.md.
type SkillMeta struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  string            `yaml:"allowed-tools,omitempty"`
}

// SkillRef represents a URL-based skill reference stored in .toc/skills.yaml.
type SkillRef struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// SkillsRegistry is the top-level structure for .toc/skills.yaml.
type SkillsRegistry struct {
	Skills []SkillRef `yaml:"skills"`
}

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("skill name must be lowercase alphanumeric with hyphens (e.g. 'code-review')")
	}
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("skill name must not end with a hyphen")
	}
	if strings.Contains(name, "--") {
		return fmt.Errorf("skill name must not contain consecutive hyphens")
	}
	return nil
}

func Dir(name string) string {
	return filepath.Join(config.SkillsDir(), name)
}

func Exists(name string) bool {
	_, err := os.Stat(Dir(name))
	return err == nil
}

// IsURL returns true if the entry looks like a URL skill reference.
// Only HTTPS URLs are accepted to prevent man-in-the-middle attacks.
func IsURL(entry string) bool {
	return strings.HasPrefix(entry, "https://")
}

// ParseSkillMD reads a SKILL.md file and extracts the YAML frontmatter.
func ParseSkillMD(path string) (*SkillMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var inFrontmatter bool
	var frontmatterLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if !inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			// Skip any content before first ---
			continue
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		frontmatterLines = append(frontmatterLines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	if len(frontmatterLines) == 0 {
		return nil, fmt.Errorf("no YAML frontmatter found in SKILL.md")
	}

	var meta SkillMeta
	if err := yaml.Unmarshal([]byte(strings.Join(frontmatterLines, "\n")), &meta); err != nil {
		return nil, fmt.Errorf("failed to parse SKILL.md frontmatter: %w", err)
	}

	return &meta, nil
}

// ValidateSkillDir checks that a directory contains a valid SKILL.md.
func ValidateSkillDir(dir string) (*SkillMeta, error) {
	skillMD := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillMD); os.IsNotExist(err) {
		return nil, fmt.Errorf("SKILL.md not found in %s", dir)
	}

	meta, err := ParseSkillMD(skillMD)
	if err != nil {
		return nil, err
	}

	if meta.Name == "" {
		return nil, fmt.Errorf("SKILL.md missing required 'name' field")
	}
	if meta.Description == "" {
		return nil, fmt.Errorf("SKILL.md missing required 'description' field")
	}
	if err := ValidateName(meta.Name); err != nil {
		return nil, fmt.Errorf("invalid skill name in SKILL.md: %w", err)
	}

	return meta, nil
}

// FindSkillMDInDir looks for a SKILL.md at the root of dir, then one level deep.
// Returns the directory containing the SKILL.md.
func FindSkillMDInDir(dir string) (string, error) {
	// Check root
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		return dir, nil
	}

	// Check one level deep
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no SKILL.md found in repository")
}

// CreateFromMeta scaffolds a new local skill in .toc/skills/<name>/ with full metadata.
func CreateFromMeta(meta SkillMeta, instructions string) error {
	if err := ValidateName(meta.Name); err != nil {
		return err
	}
	if Exists(meta.Name) {
		return fmt.Errorf("skill '%s' already exists", meta.Name)
	}

	dir := Dir(meta.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	content := renderSkillMD(meta, instructions)
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)
}

func renderSkillMD(meta SkillMeta, instructions string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", meta.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", meta.Description))
	if meta.License != "" {
		b.WriteString(fmt.Sprintf("license: %s\n", meta.License))
	}
	if meta.Compatibility != "" {
		b.WriteString(fmt.Sprintf("compatibility: %s\n", meta.Compatibility))
	}
	if len(meta.Metadata) > 0 {
		b.WriteString("metadata:\n")
		for k, v := range meta.Metadata {
			b.WriteString(fmt.Sprintf("  %s: %q\n", k, v))
		}
	}
	b.WriteString("---\n\n")

	if instructions != "" {
		b.WriteString(instructions)
		b.WriteString("\n")
	} else {
		b.WriteString(fmt.Sprintf("# %s\n\nAdd skill instructions here. These are loaded when the skill is activated.\n", meta.Name))
	}

	return b.String()
}

// Remove deletes a local skill directory.
func Remove(name string) error {
	dir := Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("skill '%s' not found", name)
	}
	return os.RemoveAll(dir)
}

// ListLocal returns metadata for all local skills in .toc/skills/.
func ListLocal() ([]SkillMeta, error) {
	entries, err := os.ReadDir(config.SkillsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []SkillMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := ValidateSkillDir(filepath.Join(config.SkillsDir(), entry.Name()))
		if err != nil {
			continue // skip invalid skills
		}
		skills = append(skills, *meta)
	}
	return skills, nil
}

// LoadRegistry reads the URL skill references from .toc/skills.yaml.
func LoadRegistry() (*SkillsRegistry, error) {
	data, err := os.ReadFile(config.SkillsRegistryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillsRegistry{}, nil
		}
		return nil, err
	}
	var reg SkillsRegistry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse skills registry: %w", err)
	}
	return &reg, nil
}

// SaveRegistry writes the URL skill references to .toc/skills.yaml.
func SaveRegistry(reg *SkillsRegistry) error {
	data, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	return os.WriteFile(config.SkillsRegistryPath(), data, 0644)
}

// AddRef adds a URL skill reference to the registry.
func AddRef(ref SkillRef) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	for _, existing := range reg.Skills {
		if existing.Name == ref.Name {
			return fmt.Errorf("skill '%s' already registered", ref.Name)
		}
	}
	reg.Skills = append(reg.Skills, ref)
	return SaveRegistry(reg)
}

// RemoveRef removes a URL skill reference from the registry.
func RemoveRef(name string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	var filtered []SkillRef
	found := false
	for _, r := range reg.Skills {
		if r.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, r)
	}
	if !found {
		return fmt.Errorf("skill '%s' not found in registry", name)
	}
	reg.Skills = filtered
	return SaveRegistry(reg)
}

// FindRef looks up a URL skill reference by name.
func FindRef(name string) (*SkillRef, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, r := range reg.Skills {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("skill '%s' not found in registry", name)
}
