package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type JsonIndexer struct {
	IndexPath string
}

func NewJsonIndexer(indexPath string) *JsonIndexer {
	return &JsonIndexer{IndexPath: indexPath}
}

func (ji *JsonIndexer) Build(notesDir string) (IndexStats, error) {
	documents := make(map[string]*DocumentMeta)
	invertedIndex := make(map[string][]string)
	facets := make(map[string]map[string]int)

	// Initialize facet maps
	facets["status"] = make(map[string]int)
	facets["confidence"] = make(map[string]int)
	facets["type"] = make(map[string]int)
	facets["scope"] = make(map[string]int)
	facets["project"] = make(map[string]int)
	facets["domain"] = make(map[string]int)
	facets["tags"] = make(map[string]int)

	// Walk notesDir recursively
	err := filepath.WalkDir(notesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip non-.md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file %s: %w", path, err)
		}

		// Parse frontmatter
		fm, body, err := ParseFrontmatter(content)
		if err != nil {
			return fmt.Errorf("parse frontmatter %s: %w", path, err)
		}

		// Generate docID as relative path from notesDir
		docID, err := filepath.Rel(notesDir, path)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", path, err)
		}

		// Normalize path separators to forward slashes for consistency
		docID = filepath.ToSlash(docID)

		// Build DocumentMeta
		meta := &DocumentMeta{
			DocID:               docID,
			Path:                path,
			Title:               fm.Title,
			Status:              fm.Status,
			Confidence:          fm.Confidence,
			Type:                fm.Type,
			Scope:               fm.Scope,
			Project:             fm.Project,
			SourceAgent:         fm.SourceAgent,
			SourceArtifact:      fm.SourceArtifact,
			RequiresHumanReview: fm.RequiresHumanReview,
			SupersededBy:        fm.SupersededBy,
			Tags:                fm.Tags,
		}

		// Handle date fields
		if created, ok := fm.Created.(string); ok && created != "" {
			meta.Created = created
		}
		if updated, ok := fm.Updated.(string); ok && updated != "" {
			meta.Updated = updated
		}
		if reviewBy, ok := fm.ReviewBy.(string); ok && reviewBy != "" {
			meta.ReviewBy = reviewBy
		}

		// Handle domain
		if fm.Domain != nil {
			if domainSlice, ok := fm.Domain.([]string); ok {
				meta.Domain = domainSlice
			}
		}

		// Collect text to tokenize: title + body
		text := fm.Title + " " + body

		// Tokenize
		tokens := Tokenize(text)

		// Add to inverted index and collect unique tokens
		// Use a seen map to deduplicate per-document token→docID entries
		seen := make(map[string]bool)
		for _, token := range tokens {
			if seen[token] {
				continue
			}
			seen[token] = true
			invertedIndex[token] = append(invertedIndex[token], docID)
		}
		// Update facets
		if meta.Status != "" {
			facets["status"][meta.Status]++
		}
		if meta.Confidence != "" {
			facets["confidence"][meta.Confidence]++
		}
		if meta.Type != "" {
			facets["type"][meta.Type]++
		}
		if meta.Scope != "" {
			facets["scope"][meta.Scope]++
		}
		if meta.Project != "" {
			facets["project"][meta.Project]++
		}
		for _, domain := range meta.Domain {
			if domain != "" {
				facets["domain"][domain]++
			}
		}
		for _, tag := range meta.Tags {
			if tag != "" {
				facets["tags"][tag]++
			}
		}

		// Store meta
		documents[docID] = meta

		return nil
	})

	if err != nil {
		return IndexStats{}, fmt.Errorf("walk notes dir: %w", err)
	}

	// Build IndexFile
	indexFile := &IndexFile{
		Version:       "1",
		Built:         time.Now(),
		NotesDir:      notesDir,
		Documents:     documents,
		InvertedIndex: invertedIndex,
		Facets:        facets,
		Stats: IndexStats{
			FileCount: len(documents),
			TermCount: len(invertedIndex),
		},
	}

	// Create parent directories if needed
	indexDir := filepath.Dir(ji.IndexPath)
	if indexDir != "." && indexDir != "" {
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			return IndexStats{}, fmt.Errorf("create index directory: %w", err)
		}
	}

	// Marshal to JSON with indent
	jsonData, err := json.MarshalIndent(indexFile, "", "  ")
	if err != nil {
		return IndexStats{}, fmt.Errorf("marshal index: %w", err)
	}
	// Set IndexBytes based on marshalled size
	indexFile.Stats.IndexBytes = int64(len(jsonData))

	// Re-marshal with IndexBytes now set
	jsonData, err = json.MarshalIndent(indexFile, "", "  ")
	if err != nil {
		return IndexStats{}, fmt.Errorf("marshal index: %w", err)
	}
	// Write to file
	if err := os.WriteFile(ji.IndexPath, jsonData, 0644); err != nil {
		return IndexStats{}, fmt.Errorf("write index file: %w", err)
	}

	return indexFile.Stats, nil
}
func (ji *JsonIndexer) Search(query Query) ([]Result, error) {
	return nil, errors.New("not implemented")
}

func (ji *JsonIndexer) Probe(field, value string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ji *JsonIndexer) Get(path string) (*Note, error) {
	return nil, errors.New("not implemented")
}
