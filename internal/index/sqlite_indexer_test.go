package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteIndexer_Build(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create 3 sample .md files with frontmatter
	notes := []struct {
		filename string
		content  string
	}{
		{
			"note1.md",
			`---
title: SQLite Notes
status: active
type: note
tags: [go, sqlite]
---
This is note 1 about SQLite.
`,
		},
		{
			"note2.md",
			`---
title: Go Patterns
status: draft
type: log
tags: [go]
superseded-by: note1.md
---
This is note 2 about Go patterns.
`,
		},
		{
			"note3.md",
			`---
title: Rust Guide
status: active
type: note
tags: [rust]
domain: [systems, languages]
---
This is note 3 about Rust.
`,
		},
	}

	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	for _, note := range notes {
		path := filepath.Join(notesDir, note.filename)
		if err := os.WriteFile(path, []byte(note.content), 0644); err != nil {
			t.Fatalf("write %s: %v", note.filename, err)
		}
	}

	// Build index
	indexer := NewSQLiteIndexer(dbPath)
	stats, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if stats.FileCount != 3 {
		t.Errorf("expected FileCount=3, got %d", stats.FileCount)
	}
	if stats.IndexBytes == 0 {
		t.Errorf("expected IndexBytes > 0, got %d", stats.IndexBytes)
	}

	// Verify database was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestSQLiteIndexer_Search(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create sample files
	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	content := `---
title: SQLite Notes
status: active
type: note
tags: [go, sqlite]
---
This is note about SQLite and databases.
`
	if err := os.WriteFile(filepath.Join(notesDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	// Build index
	indexer := NewSQLiteIndexer(dbPath)
	_, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	tests := []struct {
		name     string
		query    string
		opts     SearchOptions
		wantHits int
		wantErr  bool
	}{
		{
			name:     "empty query",
			query:    "",
			opts:     SearchOptions{},
			wantHits: 1,
			wantErr:  false,
		},
		{
			name:     "text search",
			query:    "sqlite",
			opts:     SearchOptions{},
			wantHits: 1,
			wantErr:  false,
		},
		{
			name:     "text search no results",
			query:    "nonexistent",
			opts:     SearchOptions{},
			wantHits: 0,
			wantErr:  false,
		},
		{
			name:     "with status filter",
			query:    "",
			opts:     SearchOptions{Status: "active"},
			wantHits: 1,
			wantErr:  false,
		},
		{
			name:     "with type filter",
			query:    "",
			opts:     SearchOptions{Type: "note"},
			wantHits: 1,
			wantErr:  false,
		},
		{
			name:     "with limit",
			query:    "",
			opts:     SearchOptions{Limit: 1},
			wantHits: 1,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := indexer.Search(tt.query, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got error=%v", tt.wantErr, err)
			}
			if resp.Total != tt.wantHits {
				t.Errorf("expected %d hits, got %d", tt.wantHits, resp.Total)
			}
		})
	}
}

func TestSQLiteIndexer_Probe(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	notes := []struct {
		filename string
		content  string
	}{
		{
			"note1.md",
			`---
title: Note 1
status: active
type: note
---
Content 1
`,
		},
		{
			"note2.md",
			`---
title: Note 2
status: draft
type: log
---
Content 2
`,
		},
	}

	for _, note := range notes {
		path := filepath.Join(notesDir, note.filename)
		if err := os.WriteFile(path, []byte(note.content), 0644); err != nil {
			t.Fatalf("write %s: %v", note.filename, err)
		}
	}

	indexer := NewSQLiteIndexer(dbPath)
	_, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Test probe returns facets
	result, err := indexer.Probe("", ProbeOptions{})
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	if result.TotalMatches != 2 {
		t.Errorf("expected TotalMatches=2, got %d", result.TotalMatches)
	}

	// Should have status and type facets
	if _, hasStatus := result.Facets["status"]; !hasStatus {
		t.Errorf("expected status facet")
	}
	if _, hasType := result.Facets["type"]; !hasType {
		t.Errorf("expected type facet")
	}
}

func TestSQLiteIndexer_Get(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	content := `---
title: Test Note
status: active
type: note
tags: [go, test]
---
Test body content here.
`
	if err := os.WriteFile(filepath.Join(notesDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	indexer := NewSQLiteIndexer(dbPath)
	_, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Test Get by path
	result, err := indexer.Get("test.md")
	if err != nil {
		t.Fatalf("Get by path failed: %v", err)
	}

	if result.Meta.Title != "Test Note" {
		t.Errorf("expected title 'Test Note', got '%s'", result.Meta.Title)
	}
	if result.Meta.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", result.Meta.Status)
	}
	if len(result.Meta.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(result.Meta.Tags))
	}
	if !contains(result.Meta.Tags, "go") {
		t.Errorf("expected 'go' tag")
	}
	if result.Body == "" {
		t.Errorf("expected non-empty body")
	}
}

func TestSQLiteIndexer_Context(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	content := `---
title: Test Note
status: active
type: note
tags: [go]
domain: [systems]
---
Test content
`
	if err := os.WriteFile(filepath.Join(notesDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	indexer := NewSQLiteIndexer(dbPath)
	_, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	result, err := indexer.Context()
	if err != nil {
		t.Fatalf("Context failed: %v", err)
	}

	if result.Schema == nil {
		t.Errorf("expected schema")
	}
	if result.Workflow == "" {
		t.Errorf("expected workflow")
	}
	if result.Commands == nil {
		t.Errorf("expected commands")
	}

	// Check samples
	if len(result.Samples) == 0 {
		t.Errorf("expected samples")
	}
}

func TestSQLiteIndexer_SQL(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	notesDir := filepath.Join(tempDir, "notes")
	if err := os.Mkdir(notesDir, 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	content := `---
title: Test Note
---
Test content
`
	if err := os.WriteFile(filepath.Join(notesDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	indexer := NewSQLiteIndexer(dbPath)
	_, err := indexer.Build(notesDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Test SELECT query
	result, err := indexer.SQL("SELECT count(*) FROM docs")
	if err != nil {
		t.Fatalf("SQL SELECT failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 row, got %d", len(result))
	}

	// Test non-SELECT rejected
	_, err = indexer.SQL("INSERT INTO docs VALUES ()")
	if err == nil {
		t.Errorf("expected error for non-SELECT query")
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
