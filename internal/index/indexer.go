package index

// DocumentMeta holds parsed frontmatter + file metadata
type DocumentMeta struct {
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
	LineCount           int      `json:"line_count,omitempty"`
}

// IndexStats reports build outcomes
type IndexStats struct {
	FileCount  int   `json:"file_count"`
	IndexBytes int64 `json:"index_bytes,omitempty"`
}

// SearchResult is a single result returned by Search
type SearchResult struct {
	ID         string         `json:"id"`
	Path       string         `json:"path"`
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Confidence string         `json:"confidence"`
	Type       string         `json:"type"`
	Scope      string         `json:"scope"`
	Project    string         `json:"project"`
	Domain     []string       `json:"domain"`
	Tags       []string       `json:"tags"`
	Chars      int            `json:"chars"`
	Snippet    string         `json:"snippet,omitempty"`
	Matches    map[string]int `json:"matches,omitempty"`
	Score      float64        `json:"score,omitempty"`
}

// SearchResponse wraps search results with metadata about the query.
type SearchResponse struct {
	Total   int            `json:"total"`
	Shown   int            `json:"shown"`
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

// FacetCounts maps a field value to its occurrence count
type FacetCounts map[string]int

// ProbeResult is returned by Probe
type ProbeResult struct {
	Query        string                 `json:"query"`
	TotalMatches int                    `json:"total_matches"`
	Facets       map[string]FacetCounts `json:"facets"`
}

// SearchOptions controls filtering and output for Search
type SearchOptions struct {
	Status      string
	Confidence  string
	Type        string
	Scope       string
	Project     string
	Tag         string
	Domain      string
	Limit       int
	Superseded  bool     // if false, exclude is_superseded=1 docs
	Snippets    bool
	SnippetSize int      // FTS5 token window for snippets (default 20)
	Fields      []string // extra fields beyond id+title
}

// ProbeOptions controls filtering for Probe
type ProbeOptions struct {
	Status     string
	Confidence string
	Type       string
	Scope      string
	Project    string
	Tag        string
	Domain     string
	Limit      int
	Superseded bool
}

// GetResult is returned by Get
type GetResult struct {
	ID   string       `json:"id"`
	Path string       `json:"path"`
	Meta DocumentMeta `json:"meta"`
	Body string       `json:"body,omitempty"`
}

// ContextResult is returned by Context
type ContextResult struct {
	Schema   interface{}         `json:"schema"`
	Samples  map[string][]string `json:"samples"`
	Workflow string              `json:"workflow"`
	Commands interface{}         `json:"commands"`
}
