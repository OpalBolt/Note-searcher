package get

import (
	"fmt"
	"sort"
	"strings"

	bleve "github.com/blevesearch/bleve/v2"
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

// sectionDoc is the document shape indexed into the in-memory Bleve index.
type sectionDoc struct {
	ID      string `json:"id"`
	Heading string `json:"heading"`
	Content string `json:"content"`
}

// buildSectionIndex creates an in-memory Bleve index containing one document
// per section, with "heading" and "content" fields both using the English
// analyzer (stemming + stop words).
func buildSectionIndex(headings []Heading, lines []string) (bleve.Index, error) {
	im := bleve.NewIndexMapping()
	im.DefaultAnalyzer = "en"

	idx, err := bleve.NewMemOnly(im)
	if err != nil {
		return nil, fmt.Errorf("create section index: %w", err)
	}

	b := idx.NewBatch()
	for i, h := range headings {
		doc := sectionDoc{
			ID:      h.ID,
			Heading: h.Text,
			Content: sectionContent(lines, headings, i),
		}
		if err := b.Index(h.ID, doc); err != nil {
			return nil, fmt.Errorf("index section %s: %w", h.ID, err)
		}
	}
	if err := idx.Batch(b); err != nil {
		return nil, fmt.Errorf("commit section batch: %w", err)
	}
	return idx, nil
}

// ExtractSectionsContaining returns all sections matching the Bleve query
// string (case-insensitive). Supports full Bleve query syntax:
//
//	"concepts sig"   — either word (OR, default)
//	"+concepts +sig" — both required (AND)
//	"-deprecated"    — must not contain
//	`"exact phrase"` — phrase match
//	"timeout~1"      — fuzzy match
//
// Both heading text and section content are searched.
// Returns an error if the query is invalid or no sections match.
func ExtractSectionsContaining(body, query string) ([]SectionResult, error) {
	headings := ParseHeadings(body)
	if len(headings) == 0 {
		return nil, fmt.Errorf("term not found in any section")
	}

	lines := strings.Split(body, "\n")

	idx, err := buildSectionIndex(headings, lines)
	if err != nil {
		return nil, err
	}
	defer idx.Close()

	req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(query))
	req.Size = len(headings) // return all matches
	req.Fields = []string{"*"}

	res, err := idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("section search: %w", err)
	}

	if res.Total == 0 {
		return nil, fmt.Errorf("term not found in any section")
	}

	// Collect hit IDs into a set, then return sections in file order.
	hitIDs := make(map[string]bool, len(res.Hits))
	for _, hit := range res.Hits {
		hitIDs[hit.ID] = true
	}

	// Build position map for ordering
	pos := make(map[string]int, len(headings))
	for _, h := range headings {
		pos[h.ID] = h.LineStart
	}

	var results []SectionResult
	for i, h := range headings {
		if hitIDs[h.ID] {
			results = append(results, SectionResult{
				ID:      h.ID,
				Level:   h.Level,
				Heading: h.Text,
				Content: sectionContent(lines, headings, i),
			})
		}
	}

	// Already in file order (we iterate headings in order), but sort explicitly
	// for safety in case of future refactors.
	sort.Slice(results, func(i, j int) bool {
		return pos[results[i].ID] < pos[results[j].ID]
	})

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
