package search_test

import (
	"testing"

	"github.com/OpalBolt/note-searcher/internal/index"
	"github.com/OpalBolt/note-searcher/internal/search"
)

func makeTestIndex() *index.IndexFile {
	docs := map[string]*index.DocumentMeta{
		"a.md": {DocID: "a.md", Path: "notes/a.md", Title: "Error handling", Status: "verified", Domain: []string{"golang"}, Tags: []string{"errors"}, LineCount: 100},
		"b.md": {DocID: "b.md", Path: "notes/b.md", Title: "Ingress config", Status: "inbox", Domain: []string{"kubernetes"}, Tags: []string{"ingress"}, LineCount: 200},
		"c.md": {DocID: "c.md", Path: "notes/c.md", Title: "Old pattern", Status: "deprecated", Domain: []string{"golang"}, Tags: []string{}, LineCount: 50},
		"d.md": {DocID: "d.md", Path: "notes/d.md", Title: "Superseded note", Status: "verified", SupersededBy: "a.md", LineCount: 80},
	}
	docTable := []string{"a.md", "b.md", "c.md", "d.md"}
	inv := map[string][]int{
		"error":      {0},
		"handling":   {0},
		"ingress":    {1},
		"config":     {1},
		"old":        {2},
		"pattern":    {2},
		"superseded": {3},
		"note":       {3},
	}
	return &index.IndexFile{
		DocTable:      docTable,
		Documents:     docs,
		InvertedIndex: inv,
	}
}

func TestFullTextSearch(t *testing.T) {
	e := search.NewEngine(makeTestIndex())
	results, err := e.Search(index.Query{Terms: []string{"error"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DocID != "a.md" {
		t.Errorf("expected a.md, got %v", results)
	}
}

func TestDefaultHidesDeprecated(t *testing.T) {
	e := search.NewEngine(makeTestIndex())
	results, err := e.Search(index.Query{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.DocID == "c.md" || r.DocID == "d.md" {
			t.Errorf("deprecated/superseded doc %s should be hidden", r.DocID)
		}
	}
}

func TestAllFlagShowsDeprecated(t *testing.T) {
	e := search.NewEngine(makeTestIndex())
	results, err := e.Search(index.Query{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results with --all, got %d", len(results))
	}
}

func TestMetadataFilter(t *testing.T) {
	e := search.NewEngine(makeTestIndex())
	results, err := e.Search(index.Query{
		Facets: map[string][]string{"domain": {"golang"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// c.md is golang but deprecated, should be hidden
	if len(results) != 1 || results[0].DocID != "a.md" {
		t.Errorf("expected only a.md, got %v", results)
	}
}

func TestSizeClass(t *testing.T) {
	e := search.NewEngine(makeTestIndex())
	meta := &index.DocumentMeta{LineCount: 100}
	if e.SizeClass(meta) != "small" {
		t.Error("100 lines should be small")
	}
	meta.LineCount = 200
	if e.SizeClass(meta) != "large" {
		t.Error("200 lines should be large")
	}
}
