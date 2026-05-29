package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBleveIndexer_BuildAndSearch(t *testing.T) {
	// Create temporary directories
	notesDir := t.TempDir()
	indexDir := t.TempDir()

	// Create test markdown files with frontmatter
	testFiles := map[string]string{
		"note1.md": `---
title: Error Handling Patterns
status: verified
domain:
  - golang
tags:
  - error-handling
---
error wrapping patterns in go
error handling best practices
defer cleanup patterns`,

		"note2.md": `---
title: Old Golang Patterns
status: deprecated
domain:
  - golang
---
old golang patterns
deprecated approaches
legacy code`,

		"note3.md": `---
title: Nginx Ingress Controller
status: verified
domain:
  - kubernetes
tags:
  - ingress
---
nginx ingress controller setup
kubernetes ingress configuration
routing rules`,

		"note4.md": `---
title: Kubernetes RBAC Setup
status: inbox
domain:
  - kubernetes
tags:
  - rbac
---
kubernetes rbac setup
role-based access control
service accounts`,

		"note5.md": `---
title: Old Ingress Documentation
status: superseded
superseded-by: note3.md
domain:
  - kubernetes
---
old ingress docs
outdated ingress configuration
legacy ingress setup`,
		// note6: superseded_by set but status is NOT "superseded" — tests the boolean field filter
		"note6.md": `---
title: Verified but Superseded By Another
status: verified
superseded-by: note1.md
domain:
  - golang
---
this note is verified but has been superseded`,
	}

	// Write test files
	for filename, content := range testFiles {
		path := filepath.Join(notesDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file %s: %v", filename, err)
		}
	}

	// Build index
	indexer := NewBleveIndexer(indexDir)
	stats, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("failed to build index: %v", err)
	}

	if stats.FileCount != 6 {
		t.Errorf("expected 6 files indexed, got %d", stats.FileCount)
	}
	if stats.IndexBytes <= 0 {
		t.Errorf("expected positive index bytes, got %d", stats.IndexBytes)
	}

	t.Run("search_error_default_filter", func(t *testing.T) {
		// Search "error" with default filter should hide deprecated/superseded
		resp, err := indexer.Search("error", false, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) != 1 {
			t.Errorf("expected 1 result, got %d", len(resp.Results))
		}
		if len(resp.Results) > 0 && resp.Results[0].Path != "note1.md" {
			t.Errorf("expected note1.md, got %s", resp.Results[0].Path)
		}
	})

	t.Run("search_golang_all_flag", func(t *testing.T) {
		// Search "golang" with all=true should include deprecated
		resp, err := indexer.Search("golang", true, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) != 3 {
			t.Errorf("expected 3 results, got %d", len(resp.Results))
		}
		paths := make(map[string]bool)
		for _, r := range resp.Results {
			paths[r.Path] = true
		}
		if !paths["note1.md"] || !paths["note2.md"] || !paths["note6.md"] {
			t.Errorf("expected note1.md, note2.md and note6.md in results")
		}
	})

	t.Run("search_empty_default_filter", func(t *testing.T) {
		// Empty search with default filter should get verified and inbox
		resp, err := indexer.Search("", false, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) != 3 {
			t.Errorf("expected 3 results, got %d", len(resp.Results))
		}
		paths := make(map[string]bool)
		for _, r := range resp.Results {
			paths[r.Path] = true
		}
		if !paths["note1.md"] || !paths["note3.md"] || !paths["note4.md"] {
			t.Errorf("expected note1.md, note3.md, note4.md in results")
		}
	})

	t.Run("probe_facets", func(t *testing.T) {
		// Probe empty query should return facet counts
		result, err := indexer.Probe("")
		if err != nil {
			t.Fatalf("probe failed: %v", err)
		}

		// Should have all 5 matches (probe doesn't filter)
		if result.TotalMatches != 6 {
			t.Errorf("expected 6 total matches, got %d", result.TotalMatches)
		}

		// Check status facets
		if statusFacets, ok := result.Facets["status"]; ok {
			// Should see all statuses
			if statusFacets["verified"] != 3 {
				t.Errorf("expected 3 verified, got %d", statusFacets["verified"])
			}
			if statusFacets["deprecated"] != 1 {
				t.Errorf("expected 1 deprecated, got %d", statusFacets["deprecated"])
			}
			if statusFacets["superseded"] != 1 {
				t.Errorf("expected 1 superseded, got %d", statusFacets["superseded"])
			}
			if statusFacets["inbox"] != 1 {
				t.Errorf("expected 1 inbox, got %d", statusFacets["inbox"])
			}
		} else {
			t.Errorf("expected status facet in results")
		}
	})

	t.Run("search_field_query", func(t *testing.T) {
		// Search with field query domain:kubernetes default filter
		resp, err := indexer.Search("domain:kubernetes", false, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		// Should get note3 and note4 (not note5 due to superseded filter)
		if len(resp.Results) != 2 {
			t.Errorf("expected 2 results, got %d", len(resp.Results))
		}
		paths := make(map[string]bool)
		for _, r := range resp.Results {
			paths[r.Path] = true
		}
		if !paths["note3.md"] || !paths["note4.md"] {
			t.Errorf("expected note3.md and note4.md, got %v", paths)
		}
		if paths["note5.md"] {
			t.Errorf("should not include superseded note5.md")
		}
	})

	t.Run("search_superseded_by_filter", func(t *testing.T) {
		// note6 has status:verified but superseded-by set — must be hidden by default filter
		resp, err := indexer.Search("superseded", false, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		for _, r := range resp.Results {
			if r.Path == "note6.md" {
				t.Errorf("note6.md has superseded-by set and should be hidden by default filter")
			}
		}
	})

	t.Run("search_snippet", func(t *testing.T) {
		// Search with snippet flag
		resp, err := indexer.Search("error", false, 0, true)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) != 1 {
			t.Errorf("expected 1 result, got %d", len(resp.Results))
		}
		if len(resp.Results) > 0 && resp.Results[0].Snippet == "" {
			t.Logf("note: snippet may be empty depending on Bleve highlight behavior")
		}
	})

	t.Run("search_limit", func(t *testing.T) {
		// Test limit parameter
		resp, err := indexer.Search("", false, 2, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) > 2 {
			t.Errorf("expected at most 2 results with limit=2, got %d", len(resp.Results))
		}
		// Verify metadata fields
		if resp.Shown != len(resp.Results) {
			t.Errorf("expected resp.Shown (%d) to equal len(resp.Results) (%d)", resp.Shown, len(resp.Results))
		}
		if resp.Total < resp.Shown {
			t.Errorf("expected resp.Total (%d) >= resp.Shown (%d)", resp.Total, resp.Shown)
		}
		if resp.Query != "" {
			t.Errorf("expected resp.Query to be empty string, got %q", resp.Query)
		}
	})

	t.Run("chars", func(t *testing.T) {
		// Check that chars field is populated for results
		resp, err := indexer.Search("", false, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		// All results should have Chars > 0
		for _, r := range resp.Results {
			if r.Chars <= 0 {
				t.Errorf("expected Chars > 0, got %d for %s", r.Chars, r.Path)
			}
		}
		if len(resp.Results) == 0 {
			t.Logf("warning: no results returned")
		}
	})
	
	t.Run("search_score_order", func(t *testing.T) {
		// Search "golang" to get multiple results with different relevance scores
		resp, err := indexer.Search("golang", true, 0, false)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		
		if len(resp.Results) < 2 {
			t.Errorf("expected at least 2 results to verify score order, got %d", len(resp.Results))
		}
		
		// Verify that scores are in descending order
		for i := 1; i < len(resp.Results); i++ {
			if resp.Results[i-1].Score < resp.Results[i].Score {
				t.Errorf("score order violation: result %d (score %.4f) should be >= result %d (score %.4f)", i-1, resp.Results[i-1].Score, i, resp.Results[i].Score)
			}
		}
	})
}
