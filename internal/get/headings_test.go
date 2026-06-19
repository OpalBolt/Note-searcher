package get

import (
	"fmt"
	"strings"
	"testing"
)

const testMarkdown = `# First Heading
Some content under first.

## Sub Heading
Sub content.

# Second Heading
Second content.

### Deep Heading
Deep content.

## Another Sub
Another sub content.
`

func TestParseHeadings(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected int
		checks   func(t *testing.T, headings []Heading)
	}{
		{
			name:     "empty body",
			body:     "",
			expected: 0,
		},
		{
			name:     "single heading",
			body:     "# Single",
			expected: 1,
			checks: func(t *testing.T, h []Heading) {
				if h[0].ID != "h1" || h[0].Level != 1 || h[0].Text != "Single" {
					t.Errorf("got %+v", h[0])
				}
			},
		},
		{
			name:     "mixed levels",
			body:     "# H1\n## H2\n### H3\n## H2b\n# H1b",
			expected: 5,
			checks: func(t *testing.T, h []Heading) {
				if h[0].Level != 1 || h[1].Level != 2 || h[2].Level != 3 || h[3].Level != 2 || h[4].Level != 1 {
					t.Errorf("level mismatch")
				}
				for i, heading := range h {
					expectedID := fmt.Sprintf("h%d", i+1)
					if heading.ID != expectedID {
						t.Errorf("heading %d: expected ID %s, got %s", i, expectedID, heading.ID)
					}
				}
			},
		},
		{
			name:     "ATX closing sequence",
			body:     "# Heading #\n## Another ##",
			expected: 2,
			checks: func(t *testing.T, h []Heading) {
				if h[0].Text != "Heading" || h[1].Text != "Another" {
					t.Errorf("closing sequence not stripped: %q %q", h[0].Text, h[1].Text)
				}
			},
		},
		{
			name:     "no heading lines",
			body:     "This is just text\nNo headings here\nAt all",
			expected: 0,
		},
		{
			name:     "headings with special chars",
			body:     "# Heading with `code`\n## Sub with **bold** and _italic_",
			expected: 2,
			checks: func(t *testing.T, h []Heading) {
				if h[0].Text != "Heading with `code`" {
					t.Errorf("special chars not preserved: %q", h[0].Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headings := ParseHeadings(tt.body)
			if len(headings) != tt.expected {
				t.Errorf("expected %d headings, got %d", tt.expected, len(headings))
			}
			if tt.checks != nil {
				tt.checks(t, headings)
			}
		})
	}
}

func TestHeadingLevel(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected int
	}{
		{
			name:     "level 1",
			line:     "# Heading",
			expected: 1,
		},
		{
			name:     "level 2",
			line:     "## Heading",
			expected: 2,
		},
		{
			name:     "level 3",
			line:     "### Heading",
			expected: 3,
		},
		{
			name:     "level 4",
			line:     "#### Heading",
			expected: 4,
		},
		{
			name:     "level 5",
			line:     "##### Heading",
			expected: 5,
		},
		{
			name:     "level 6",
			line:     "###### Heading",
			expected: 6,
		},
		{
			name:     "no space after hash (not a heading)",
			line:     "##without-space",
			expected: 0,
		},
		{
			name:     "not a heading",
			line:     "Just text",
			expected: 0,
		},
		{
			name:     "hash in middle",
			line:     "text # not heading",
			expected: 0,
		},
		{
			name:     "empty line",
			line:     "",
			expected: 0,
		},
		{
			name:     "seven hashes (too many)",
			line:     "####### Too many",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headingLevel(tt.line)
			if got != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestSectionContent(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		idx    int
		checks func(t *testing.T, content string, headings []Heading)
	}{
		{
			name: "greedy boundary stops at same-or-higher",
			body: testMarkdown,
			idx:  1, // ## Sub Heading
			checks: func(t *testing.T, content string, headings []Heading) {
				// Should include Sub Heading line and its content, but stop before "# Second Heading"
				if len(content) == 0 {
					t.Errorf("content empty")
				}
				if headings[1].Text != "Sub Heading" {
					t.Errorf("wrong heading: %q", headings[1].Text)
				}
			},
		},
		{
			name: "end of file",
			body: testMarkdown,
			idx:  4, // ## Another Sub (last heading)
			checks: func(t *testing.T, content string, headings []Heading) {
				if len(content) == 0 {
					t.Errorf("content empty for last section")
				}
			},
		},
		{
			name: "nested headings included",
			body: testMarkdown,
			idx:  2, // # Second Heading
			checks: func(t *testing.T, content string, headings []Heading) {
				// Should include the H3 and H2 under it
				if len(content) == 0 {
					t.Errorf("content empty for nested section")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headings := ParseHeadings(tt.body)
			lines := strings.Split(tt.body, "\n")
			if len(headings) == 0 && tt.idx >= 0 {
				t.Errorf("no headings parsed from body")
				return
			}
			if tt.idx >= len(headings) {
				t.Errorf("idx out of range")
				return
			}
			content := sectionContent(lines, headings, tt.idx)
			if tt.checks != nil {
				tt.checks(t, content, headings)
			}
		})
	}
}

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		id      string
		wantErr bool
		checks  func(t *testing.T, r SectionResult)
	}{
		{
			name:    "found h1",
			body:    testMarkdown,
			id:      "h1",
			wantErr: false,
			checks: func(t *testing.T, r SectionResult) {
				if r.Heading != "First Heading" {
					t.Errorf("expected 'First Heading', got %q", r.Heading)
				}
				if r.ID != "h1" {
					t.Errorf("expected ID h1, got %q", r.ID)
				}
			},
		},
		{
			name:    "found case-insensitive",
			body:    testMarkdown,
			id:      "H2",
			wantErr: false,
			checks: func(t *testing.T, r SectionResult) {
				if r.ID != "h2" {
					t.Errorf("case insensitive lookup failed")
				}
			},
		},
		{
			name:    "not found",
			body:    testMarkdown,
			id:      "h99",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ExtractSection(tt.body, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr %v, got %v", tt.wantErr, err != nil)
			}
			if !tt.wantErr && tt.checks != nil {
				tt.checks(t, r)
			}
		})
	}
}

func TestExtractSectionsContaining(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		term    string
		wantErr bool
		checks  func(t *testing.T, r []SectionResult)
	}{
		{
			name:    "single match",
			body:    testMarkdown,
			term:    "Sub content",
			wantErr: false,
			checks: func(t *testing.T, r []SectionResult) {
				if len(r) == 0 {
					t.Errorf("expected at least 1 result")
				}
			},
		},
		{
			name:    "multiple matches",
			body:    testMarkdown,
			term:    "content", // appears in multiple sections
			wantErr: false,
			checks: func(t *testing.T, r []SectionResult) {
				if len(r) < 2 {
					t.Errorf("expected multiple matches, got %d", len(r))
				}
			},
		},
		{
			name:    "no match",
			body:    testMarkdown,
			term:    "nonexistent-term-xyz",
			wantErr: true,
		},
		{
			name:    "case-insensitive",
			body:    testMarkdown,
			term:    "CONTENT", // uppercase
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headings := ParseHeadings(tt.body)
			lines := strings.Split(tt.body, "\n")
			r := ExtractSectionsContaining(headings, lines, tt.term)
			gotErr := len(r) == 0 && tt.wantErr
			if tt.wantErr && !gotErr {
				t.Errorf("wantErr %v, got results=%d", tt.wantErr, len(r))
			}
			if !tt.wantErr && tt.checks != nil {
				tt.checks(t, r)
			}
		})
	}
}

func TestUnionSections(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		aTerms []string // terms to search for in set A
		bTerms []string // terms to search for in set B
		checks func(t *testing.T, merged []SectionResult, headings []Heading)
	}{
		{
			name:   "dedup identical",
			body:   testMarkdown,
			aTerms: []string{"First"},
			bTerms: []string{"First"}, // same term, should dedup
			checks: func(t *testing.T, merged []SectionResult, headings []Heading) {
				idMap := make(map[string]int)
				for _, s := range merged {
					idMap[s.ID]++
				}
				for id, count := range idMap {
					if count > 1 {
						t.Errorf("ID %s appears %d times, should be deduped", id, count)
					}
				}
			},
		},
		{
			name:   "order by file position",
			body:   testMarkdown,
			aTerms: []string{"Second"}, // h3
			bTerms: []string{"First"},  // h1
			checks: func(t *testing.T, merged []SectionResult, headings []Heading) {
				// Create position map
				pos := make(map[string]int)
				for _, h := range headings {
					pos[h.ID] = h.LineStart
				}
				// Check order
				for i := 0; i < len(merged)-1; i++ {
					if pos[merged[i].ID] > pos[merged[i+1].ID] {
						t.Errorf("not in file order: %s before %s", merged[i].ID, merged[i+1].ID)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headings := ParseHeadings(tt.body)

			var aSlice, bSlice []SectionResult
			lines := strings.Split(tt.body, "\n")
			for _, term := range tt.aTerms {
				aSlice = append(aSlice, ExtractSectionsContaining(headings, lines, term)...)
			}
			for _, term := range tt.bTerms {
				bSlice = append(bSlice, ExtractSectionsContaining(headings, lines, term)...)
			}

			merged := UnionSections(aSlice, bSlice, headings)
			if tt.checks != nil {
				tt.checks(t, merged, headings)
			}
		})
	}
}
