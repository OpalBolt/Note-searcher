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
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Domain  []string `json:"domain"`
	Tags    []string `json:"tags"`
	Chars   int      `json:"chars"`
	Snippet  string   `json:"snippet,omitempty"`
	Matches  int      `json:"matches,omitempty"`
	Score    float64  `json:"score,omitempty"`
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
