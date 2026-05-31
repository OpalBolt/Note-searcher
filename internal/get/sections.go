package get

import (
	"fmt"
	"sort"
	"strings"
)

// SectionResult holds one extracted section (heading + content).
type SectionResult struct {
	ID      string `json:"id"`
	Level   int    `json:"level"`
	Heading string `json:"heading"`
	Content string `json:"content"`
}

// sectionContent extracts the raw lines for the section at headings[idx],
// using greedy boundary: from headings[idx].LineStart to the line before the
// next heading whose level <= headings[idx].Level (or end of file).
func sectionContent(lines []string, headings []Heading, idx int) string {
	start := headings[idx].LineStart
	end := len(lines)
	for j := idx + 1; j < len(headings); j++ {
		if headings[j].Level <= headings[idx].Level {
			end = headings[j].LineStart
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// ExtractSection returns the section matching id (case-insensitive).
// Returns an error if the id is not found.
func ExtractSection(body, id string) (SectionResult, error) {
	headings := ParseHeadings(body)
	lines := strings.Split(body, "\n")
	idLower := strings.ToLower(id)
	for i, h := range headings {
		if strings.ToLower(h.ID) == idLower {
			return SectionResult{
				ID:      h.ID,
				Level:   h.Level,
				Heading: h.Text,
				Content: sectionContent(lines, headings, i),
			}, nil
		}
	}
	return SectionResult{}, fmt.Errorf("section not found")
}

// ExtractSectionsContaining returns all sections whose content contains term
// (case-insensitive). Returns an error if no sections match.
func ExtractSectionsContaining(body, term string) ([]SectionResult, error) {
	headings := ParseHeadings(body)
	lines := strings.Split(body, "\n")
	termLower := strings.ToLower(term)
	var results []SectionResult
	for i, h := range headings {
		content := sectionContent(lines, headings, i)
		if strings.Contains(strings.ToLower(content), termLower) {
			results = append(results, SectionResult{
				ID:      h.ID,
				Level:   h.Level,
				Heading: h.Text,
				Content: content,
			})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("term not found in any section")
	}
	return results, nil
}

// UnionSections merges two SectionResult slices, deduplicates by ID,
// and returns results in file order (by heading position).
func UnionSections(a, b []SectionResult, headings []Heading) []SectionResult {
	// Build position map: ID -> LineStart
	pos := make(map[string]int, len(headings))
	for _, h := range headings {
		pos[h.ID] = h.LineStart
	}

	seen := make(map[string]bool)
	var merged []SectionResult
	for _, s := range append(a, b...) {
		if !seen[s.ID] {
			seen[s.ID] = true
			merged = append(merged, s)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return pos[merged[i].ID] < pos[merged[j].ID]
	})
	return merged
}
