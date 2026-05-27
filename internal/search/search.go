package search

import (
	"strings"

	"github.com/OpalBolt/note-searcher/internal/index"
)

// Engine performs searches against a loaded IndexFile.
type Engine struct {
	idx       *index.IndexFile
	threshold int // line count threshold for size classification
}

func NewEngine(idx *index.IndexFile) *Engine {
	return &Engine{idx: idx, threshold: 150}
}

func (e *Engine) WithThreshold(t int) *Engine {
	e.threshold = t
	return e
}

// Search executes a query against the index and returns matching results.
func (e *Engine) Search(q index.Query) ([]index.Result, error) {
	// Step 1: full-text — find candidate docIDs
	var candidates []int
	if len(q.Terms) > 0 {
		candidates = e.intersect(q.Terms)
	} else {
		// no text query — start with all docs
		candidates = e.allDocIDs()
	}

	// Step 2: apply metadata filters (OR within key, AND across keys)
	if len(q.Facets) > 0 {
		candidates = e.filterMeta(candidates, q.Facets)
	}

	// Step 3: default hide unless All=true
	if !q.All {
		candidates = e.hideDeprecated(candidates)
	}

	// Step 4: build results
	results := make([]index.Result, 0, len(candidates))
	for _, id := range candidates {
		if id >= len(e.idx.DocTable) {
			continue
		}
		docID := e.idx.DocTable[id]
		meta, ok := e.idx.Documents[docID]
		if !ok {
			continue
		}
		results = append(results, index.Result{
			DocID: docID,
			Score: 1.0,
			Meta:  meta,
		})
	}
	return results, nil
}

// intersect returns docIDs that appear in ALL term posting lists (AND semantics).
func (e *Engine) intersect(terms []string) []int {
	var result []int
	first := true
	for _, term := range terms {
		lower := strings.ToLower(term)
		posting, ok := e.idx.InvertedIndex[lower]
		if !ok {
			return nil // term not in index → empty result
		}
		if first {
			result = make([]int, len(posting))
			copy(result, posting)
			first = false
		} else {
			result = intersectSorted(result, posting)
		}
		if len(result) == 0 {
			return nil
		}
	}
	return result
}

// intersectSorted returns the intersection of two sorted int slices.
func intersectSorted(a, b []int) []int {
	out := make([]int, 0, min(len(a), len(b)))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			out = append(out, a[i])
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (e *Engine) allDocIDs() []int {
	ids := make([]int, len(e.idx.DocTable))
	for i := range ids {
		ids[i] = i
	}
	return ids
}

// filterMeta applies metadata filters: OR within same key, AND across keys.
// All comparisons are case-insensitive.
func (e *Engine) filterMeta(candidates []int, facets map[string][]string) []int {
	out := candidates[:0:0] // reuse backing array intent but fresh
	out = append(out, candidates...)
	for key, values := range facets {
		if len(values) == 0 {
			continue
		}
		// normalise filter values to lowercase
		lower := make([]string, len(values))
		for i, v := range values {
			lower[i] = strings.ToLower(v)
		}
		filtered := out[:0]
		for _, id := range out {
			if id >= len(e.idx.DocTable) {
				continue
			}
			docID := e.idx.DocTable[id]
			meta, ok := e.idx.Documents[docID]
			if !ok {
				continue
			}
			if matchesFacet(meta, key, lower) {
				filtered = append(filtered, id)
			}
		}
		out = filtered
	}
	return out
}

// matchesFacet returns true if meta matches any of the lowercased values for the given facet key.
func matchesFacet(meta *index.DocumentMeta, key string, values []string) bool {
	switch key {
	case "status":
		return containsCI(values, meta.Status)
	case "type":
		return containsCI(values, meta.Type)
	case "confidence":
		return containsCI(values, meta.Confidence)
	case "scope":
		return containsCI(values, meta.Scope)
	case "project":
		return containsCI(values, meta.Project)
	case "domain":
		for _, d := range meta.Domain {
			if containsCI(values, d) {
				return true
			}
		}
		return false
	case "tags":
		for _, t := range meta.Tags {
			if containsCI(values, t) {
				return true
			}
		}
		return false
	}
	return false
}

func containsCI(haystack []string, needle string) bool {
	lower := strings.ToLower(needle)
	for _, h := range haystack {
		if h == lower {
			return true
		}
	}
	return false
}

// hideDeprecated removes docs with status=deprecated, status=superseded, or non-empty superseded-by.
func (e *Engine) hideDeprecated(candidates []int) []int {
	out := candidates[:0:0]
	out = append(out, candidates...)
	filtered := out[:0]
	for _, id := range out {
		if id >= len(e.idx.DocTable) {
			continue
		}
		docID := e.idx.DocTable[id]
		meta, ok := e.idx.Documents[docID]
		if !ok {
			continue
		}
		s := strings.ToLower(meta.Status)
		if s == "deprecated" || s == "superseded" {
			continue
		}
		if meta.SupersededBy != "" {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

// SizeClass returns "small" or "large" based on line count and threshold.
func (e *Engine) SizeClass(meta *index.DocumentMeta) string {
	if meta.LineCount > e.threshold {
		return "large"
	}
	return "small"
}
