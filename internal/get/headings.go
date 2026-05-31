package get

import (
	"fmt"
	"strings"

	// Register the English analyzer used by ExtractSectionsContaining.
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
)

// Heading represents a single markdown heading extracted from a document body.
type Heading struct {
	ID        string `json:"id"`    // sequential: "h1", "h2", ...
	Level     int    `json:"level"` // 1–6
	Text      string `json:"text"`  // heading text without # chars
	LineStart int    `json:"-"`     // 0-indexed line number in body (not in JSON output)
}

// ParseHeadings scans body (markdown text, no frontmatter) and returns all
// H1–H6 headings in file order with sequential IDs h1, h2, ...
func ParseHeadings(body string) []Heading {
	lines := strings.Split(body, "\n")
	var headings []Heading
	counter := 0
	for i, line := range lines {
		level := headingLevel(line)
		if level == 0 {
			continue
		}
		counter++
		text := strings.TrimSpace(line[level:])
		// strip trailing # chars (ATX closing sequence)
		text = strings.TrimRight(text, "# ")
		text = strings.TrimSpace(text)
		headings = append(headings, Heading{
			ID:        fmt.Sprintf("h%d", counter),
			Level:     level,
			Text:      text,
			LineStart: i,
		})
	}
	return headings
}

// headingLevel returns the ATX heading level (1–6) for a line, or 0 if not a heading.
func headingLevel(line string) int {
	if len(line) == 0 || line[0] != '#' {
		return 0
	}
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	// Must be followed by a space (or be end of line for empty heading)
	if level < len(line) && line[level] != ' ' {
		return 0
	}
	return level
}
