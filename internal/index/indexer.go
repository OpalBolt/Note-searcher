package index

import (
	"encoding/json"
	"time"
)

// DocumentMeta holds parsed frontmatter + file path
type DocumentMeta struct {
	DocID               string   `json:"doc_id"`
	Path                string   `json:"path"`
	Title               string   `json:"title"`
	Created             string   `json:"created,omitempty"`
	Updated             string   `json:"updated,omitempty"`
	Status              string   `json:"status,omitempty"`
	Confidence          string   `json:"confidence,omitempty"`
	Type                string   `json:"type,omitempty"`
	Scope               string   `json:"scope,omitempty"`
	Project             string   `json:"project,omitempty"`
	Domain              []string `json:"domain,omitempty"`
	SourceAgent         string   `json:"source_agent,omitempty"`
	SourceArtifact      string   `json:"source_artifact,omitempty"`
	ReviewBy            string   `json:"review_by,omitempty"`
	RequiresHumanReview bool     `json:"requires_human_review,omitempty"`
	SupersededBy        string   `json:"superseded_by,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

// IndexFile is the on-disk representation
type IndexFile struct {
	Version       string                    `json:"version"`
	Built         time.Time                 `json:"built"`
	NotesDir      string                    `json:"notes_dir"`
	Documents     map[string]*DocumentMeta  `json:"documents"`      // docID -> meta
	InvertedIndex map[string][]string       `json:"inverted_index"` // term -> []docID
	Facets        map[string]map[string]int `json:"facets"`         // field -> value -> count
	Stats         IndexStats                `json:"stats"`
}

type IndexStats struct {
	FileCount  int   `json:"file_count"`
	TermCount  int   `json:"term_count"`
	IndexBytes int64 `json:"index_bytes"`
}

// Query is a placeholder for future search
type Query struct {
	Terms  []string
	Facets map[string]string
}

// Result is a placeholder for future search
type Result struct {
	DocID string
	Score float64
	Meta  *DocumentMeta
}

// Note is a placeholder for future get
type Note struct {
	Meta    *DocumentMeta
	Content string
}

// Indexer is the pluggable interface
type Indexer interface {
	Build(notesDir string) (IndexStats, error)
	Search(query Query) ([]Result, error)
	Probe(field, value string) ([]string, error)
	Get(path string) (*Note, error)
}

// UnmarshalIndex unmarshals JSON bytes into an IndexFile struct
func UnmarshalIndex(data []byte, indexFile *IndexFile) error {
	return json.Unmarshal(data, indexFile)
}
