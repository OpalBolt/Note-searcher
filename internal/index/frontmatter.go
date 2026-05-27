package index

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds the raw YAML fields from a markdown file
type Frontmatter struct {
	Title               string      `yaml:"title"`
	Created             interface{} `yaml:"created"` // may be date or string
	Updated             interface{} `yaml:"updated"`
	Status              string      `yaml:"status"`
	Confidence          string      `yaml:"confidence"`
	Type                string      `yaml:"type"`
	Scope               string      `yaml:"scope"`
	Project             string      `yaml:"project"`
	Domain              interface{} `yaml:"domain"` // may be []string or string
	SourceAgent         string      `yaml:"source-agent"`
	SourceArtifact      string      `yaml:"source-artifact"`
	ReviewBy            interface{} `yaml:"review-by"`
	RequiresHumanReview bool        `yaml:"requires-human-review"`
	SupersededBy        string      `yaml:"superseded-by"`
	Tags                []string    `yaml:"tags"`
}

// ParseFrontmatter splits a markdown file into frontmatter and body.
// Returns (frontmatter, body, error).
// If no frontmatter found, returns empty Frontmatter and full content as body.
func ParseFrontmatter(content []byte) (Frontmatter, string, error) {
	fm := Frontmatter{}

	// Check if content starts with ---\n
	if !bytes.HasPrefix(content, []byte("---\n")) {
		// No frontmatter, return empty frontmatter and full content as body
		return fm, string(content), nil
	}

	// Find closing ---\n
	remainder := content[4:] // Skip opening ---\n
	closingIdx := bytes.Index(remainder, []byte("\n---\n"))
	if closingIdx == -1 {
		// No closing delimiter found, treat whole content as body
		return fm, string(content), nil
	}

	// Extract frontmatter YAML and body
	yamlContent := remainder[:closingIdx]
	bodyStart := closingIdx + 5 // Skip \n---\n
	body := string(remainder[bodyStart:])

	// Unmarshal YAML
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return fm, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	// Normalize domain: convert string to []string
	if fm.Domain != nil {
		switch v := fm.Domain.(type) {
		case string:
			if v != "" {
				fm.Domain = []string{v}
			} else {
				fm.Domain = []string{}
			}
		case []interface{}:
			domains := make([]string, len(v))
			for i, d := range v {
				domains[i] = fmt.Sprintf("%v", d)
			}
			fm.Domain = domains
		}
	}

	// Convert date fields to string format "2006-01-02"
	fm.Created = formatDateField(fm.Created)
	fm.Updated = formatDateField(fm.Updated)
	fm.ReviewBy = formatDateField(fm.ReviewBy)

	return fm, body, nil
}

// formatDateField converts a date field (which may be time.Time or string) to "2006-01-02" format
func formatDateField(field interface{}) interface{} {
	if field == nil {
		return ""
	}

	switch v := field.(type) {
	case time.Time:
		return v.Format("2006-01-02")
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
