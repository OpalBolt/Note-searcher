package index

import (
	"fmt"
	"os"
	"path/filepath"

	bleve "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	bquery "github.com/blevesearch/bleve/v2/search/query"

	// Register the English analyzer
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
)

// BleveIndexer is the sole index backend.
type BleveIndexer struct {
	IndexPath string
}

func NewBleveIndexer(indexPath string) *BleveIndexer {
	return &BleveIndexer{IndexPath: indexPath}
}

// buildMapping creates the index mapping:
//   - "body" and "title": English analyzer (stemming + stop words)
//   - metadata fields: keyword analyzer (exact match, case-folded)
//   - "is_superseded": boolean (true when superseded-by is non-empty)
func buildMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()
	im.DefaultAnalyzer = "keyword"

	englishField := bleve.NewTextFieldMapping()
	englishField.Analyzer = "en"

	keywordField := bleve.NewTextFieldMapping()
	keywordField.Analyzer = "keyword"

	numericField := bleve.NewNumericFieldMapping()
	boolField := bleve.NewBooleanFieldMapping()

	dm := bleve.NewDocumentMapping()
	dm.AddFieldMappingsAt("body", englishField)
	dm.AddFieldMappingsAt("title", englishField)
	dm.AddFieldMappingsAt("path", keywordField)
	dm.AddFieldMappingsAt("status", keywordField)
	dm.AddFieldMappingsAt("confidence", keywordField)
	dm.AddFieldMappingsAt("type", keywordField)
	dm.AddFieldMappingsAt("scope", keywordField)
	dm.AddFieldMappingsAt("project", keywordField)
	dm.AddFieldMappingsAt("domain", keywordField)
	dm.AddFieldMappingsAt("tags", keywordField)
	dm.AddFieldMappingsAt("source_agent", keywordField)
	dm.AddFieldMappingsAt("superseded_by", keywordField)
	dm.AddFieldMappingsAt("requires_human_review", boolField)
	dm.AddFieldMappingsAt("is_superseded", boolField)
	dm.AddFieldMappingsAt("chars", numericField)

	im.DefaultMapping = dm
	return im
}

// Build walks notesDir, parses frontmatter, and indexes all .md files.
func (b *BleveIndexer) Build(notesDir string) (IndexStats, error) {
	if err := os.RemoveAll(b.IndexPath); err != nil {
		return IndexStats{}, fmt.Errorf("remove old index: %w", err)
	}

	idx, err := bleve.New(b.IndexPath, buildMapping())
	if err != nil {
		return IndexStats{}, fmt.Errorf("create bleve index: %w", err)
	}
	defer idx.Close()

	batch := idx.NewBatch()
	fileCount := 0

	err = filepath.WalkDir(notesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}

		fm, body, err := ParseFrontmatter(content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}

		charCount := len([]rune(string(content)))

		var domain []string
		if fm.Domain != nil {
			if ds, ok := fm.Domain.([]string); ok {
				domain = ds
			}
		}
		if domain == nil {
			domain = []string{}
		}

		tags := fm.Tags
		if tags == nil {
			tags = []string{}
		}

		isSuperseded := fm.SupersededBy != ""

		doc := map[string]interface{}{
			"path":                  path,
			"body":                  body,
			"title":                 fm.Title,
			"status":                fm.Status,
			"confidence":            fm.Confidence,
			"type":                  fm.Type,
			"scope":                 fm.Scope,
			"project":               fm.Project,
			"domain":                domain,
			"tags":                  tags,
			"source_agent":          fm.SourceAgent,
			"superseded_by":         fm.SupersededBy,
			"requires_human_review": fm.RequiresHumanReview,
			"is_superseded":         isSuperseded,
			"chars":                 float64(charCount),
		}

		docID, err := filepath.Rel(notesDir, path)
		if err != nil {
			docID = path
		}
		docID = filepath.ToSlash(docID)
		// Store relative path so results are portable across machines
		doc["path"] = docID

		if err := batch.Index(docID, doc); err != nil {
			fmt.Fprintf(os.Stderr, "warning: index %s: %v\n", path, err)
			return nil
		}
		fileCount++

		// Flush every 500 docs to bound memory usage
		if fileCount%500 == 0 {
			if err := idx.Batch(batch); err != nil {
				return fmt.Errorf("flush batch: %w", err)
			}
			batch = idx.NewBatch()
		}

		return nil
	})
	if err != nil {
		return IndexStats{}, fmt.Errorf("walk notes dir: %w", err)
	}

	if batch.Size() > 0 {
		if err := idx.Batch(batch); err != nil {
			return IndexStats{}, fmt.Errorf("flush final batch: %w", err)
		}
	}

	return IndexStats{
		FileCount:  fileCount,
		IndexBytes: dirSize(b.IndexPath),
	}, nil
}

// Search executes a Bleve query string. Unless all=true, deprecated and superseded
// notes are excluded by default.
func (b *BleveIndexer) Search(queryStr string, all bool, limit int, snippetFlag bool) (SearchResponse, error) {
	idx, err := bleve.Open(b.IndexPath)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("open bleve index: %w", err)
	}
	defer idx.Close()

	q := buildFinalQuery(queryStr, all)

	size := 100
	if limit > 0 {
		size = limit
	}

	req := bleve.NewSearchRequestOptions(q, size, 0, false)
	req.Fields = []string{"path", "title", "status", "domain", "tags", "chars"}
	if snippetFlag {
		req.Highlight = bleve.NewHighlight()
		req.Fields = append(req.Fields, "body")
	}

	res, err := idx.Search(req)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("execute search: %w", err)
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		path := fieldString(hit.Fields["path"])
		title := fieldString(hit.Fields["title"])
		status := fieldString(hit.Fields["status"])
		domain := fieldStringSlice(hit.Fields["domain"])
		tags := fieldStringSlice(hit.Fields["tags"])
		charCount := fieldInt(hit.Fields["chars"])

		var snippet string
		if snippetFlag && hit.Fragments != nil {
			for _, frags := range hit.Fragments {
				if len(frags) > 0 {
					snippet = frags[0]
					break
				}
			}
		}

		results = append(results, SearchResult{
			Path:    path,
			Title:   title,
			Status:  status,
			Domain:  domain,
			Tags:    tags,
			Chars:   charCount,
			Snippet: snippet,
			Score:   hit.Score,
		})
	}
	return SearchResponse{
		Total:   int(res.Total),
		Shown:   len(results),
		Query:   queryStr,
		Results: results,
	}, nil
}

// Probe executes a query and returns facet counts over the matching result set.
// All docs are included (no default filter) so agents see full corpus distribution.
func (b *BleveIndexer) Probe(queryStr string) (ProbeResult, error) {
	idx, err := bleve.Open(b.IndexPath)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("open bleve index: %w", err)
	}
	defer idx.Close()

	var q bquery.Query
	if queryStr == "" {
		q = bleve.NewMatchAllQuery()
	} else {
		q = bleve.NewQueryStringQuery(queryStr)
	}

	req := bleve.NewSearchRequestOptions(q, 0, 0, false)

	for _, field := range []string{"status", "confidence", "type", "scope", "project", "domain", "tags"} {
		req.AddFacet(field, bleve.NewFacetRequest(field, 100))
	}

	res, err := idx.Search(req)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("execute probe: %w", err)
	}

	facets := make(map[string]FacetCounts)
	for field, fr := range res.Facets {
		counts := make(FacetCounts)
		for _, term := range fr.Terms.Terms() {
			counts[term.Term] = term.Count
		}
		if len(counts) > 0 {
			facets[field] = counts
		}
	}

	return ProbeResult{
		Query:        queryStr,
		TotalMatches: int(res.Total),
		Facets:       facets,
	}, nil
}

// buildFinalQuery wraps the user query with default deprecation filters when all=false.
func buildFinalQuery(queryStr string, all bool) bquery.Query {
	var userQuery bquery.Query
	if queryStr == "" {
		userQuery = bleve.NewMatchAllQuery()
	} else {
		userQuery = bleve.NewQueryStringQuery(queryStr)
	}

	if all {
		return userQuery
	}

	deprecatedQ := bleve.NewTermQuery("deprecated")
	deprecatedQ.SetField("status")

	supersededStatusQ := bleve.NewTermQuery("superseded")
	supersededStatusQ.SetField("status")

	supersededByQ := bleve.NewBoolFieldQuery(true)
	supersededByQ.SetField("is_superseded")

	boolQ := bleve.NewBooleanQuery()
	boolQ.AddMust(userQuery)
	boolQ.AddMustNot(deprecatedQ)
	boolQ.AddMustNot(supersededStatusQ)
	boolQ.AddMustNot(supersededByQ)

	return boolQ
}

// dirSize returns total byte size of all files under path.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func fieldString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []interface{}:
		if len(s) > 0 {
			if str, ok := s[0].(string); ok {
				return str
			}
		}
	}
	return fmt.Sprintf("%v", v)
}

func fieldStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	switch s := v.(type) {
	case string:
		if s == "" {
			return []string{}
		}
		return []string{s}
	case []interface{}:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return s
	}
	return []string{}
}

func fieldInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
