package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	bleve "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	bquery "github.com/blevesearch/bleve/v2/search/query"
	bleveIndexAPI "github.com/blevesearch/bleve_index_api"

	// Register the English analyzer
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
	// Register the HTML highlight formatter
	_ "github.com/blevesearch/bleve/v2/search/highlight/format/html"
	porterstemmer "github.com/blevesearch/go-porterstemmer"
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
	dm.AddFieldMappingsAt("id", keywordField)

	im.DefaultMapping = dm
	return im
}

// minUniquePrefixLen computes the minimum prefix length n (starting at 7)
// where no two IDs share an n-character prefix.
func minUniquePrefixLen(ids []string) int {
	for n := 7; n <= 64; n++ {
		seen := make(map[string]bool, len(ids))
		collision := false
		for _, id := range ids {
			if len(id) < n {
				continue
			}
			prefix := id[:n]
			if seen[prefix] {
				collision = true
				break
			}
			seen[prefix] = true
		}
		if !collision {
			return n
		}
	}
	return 64
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
		relPath := docID // relPath is the slash-normalized relative path
		h := sha256.Sum256([]byte(relPath))
		idHex := hex.EncodeToString(h[:])
		doc["id"] = idHex

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
	// Compute minimum unique prefix length and store as metadata
	q := bleve.NewMatchAllQuery()
	req := bleve.NewSearchRequestOptions(q, int(fileCount), 0, false)
	req.Fields = []string{"id"}
	res, err := idx.Search(req)
	if err == nil && res.Total > 0 {
		var idHexes []string
		for _, hit := range res.Hits {
			idHex := fieldString(hit.Fields["id"])
			if idHex != "" {
				idHexes = append(idHexes, idHex)
			}
		}
		n := minUniquePrefixLen(idHexes)
		meta := map[string]interface{}{"prefix_len": strconv.Itoa(n)}
		idx.Index("__meta__prefix_len", meta)
	}

	return IndexStats{
		FileCount:  fileCount,
		IndexBytes: dirSize(b.IndexPath),
	}, nil
}

// Search executes a Bleve query string. Unless all=true, deprecated and superseded
// notes are excluded by default.
func (b *BleveIndexer) Search(queryStr string, all bool, limit int, snippetFlag bool, snippetSize int) (SearchResponse, error) {
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
	req.Fields = []string{"path", "title", "status", "domain", "tags", "chars", "id"}
	if snippetFlag {
		req.Highlight = bleve.NewHighlight()
		htmlStyle := "html"
		req.Highlight.Style = &htmlStyle
		req.Fields = append(req.Fields, "body")
	}

	res, err := idx.Search(req)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("execute search: %w", err)
	}
	// Load cached prefix length by fetching the metadata document directly by key
	n := 7
	if metaDoc, _ := idx.Document("__meta__prefix_len"); metaDoc != nil {
		metaDoc.VisitFields(func(field bleveIndexAPI.Field) {
			if field.Name() == "prefix_len" {
				if v, err2 := strconv.Atoi(string(field.Value())); err2 == nil {
					n = v
				}
			}
		})
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		// Skip metadata documents (those with hit.ID starting with __)
		if strings.HasPrefix(hit.ID, "__") {
			continue
		}
		path := fieldString(hit.Fields["path"])
		title := fieldString(hit.Fields["title"])
		status := fieldString(hit.Fields["status"])
		domain := fieldStringSlice(hit.Fields["domain"])
		tags := fieldStringSlice(hit.Fields["tags"])
		charCount := fieldInt(hit.Fields["chars"])
		idHex := fieldString(hit.Fields["id"])
		shortID := idHex
		if len(shortID) > n {
			shortID = shortID[:n]
		}

		var snippet string
		var matches map[string]int
		if snippetFlag {
			body := fieldString(hit.Fields["body"])
			frags := hit.Fragments["body"]
			snippet = buildSnippets(body, frags, snippetSize)
			matches = countTermMatches(body, frags, queryStr)
		}

		results = append(results, SearchResult{
			ID:      shortID,
			Path:    path,
			Title:   title,
			Status:  status,
			Domain:  domain,
			Tags:    tags,
			Chars:   charCount,
			Snippet: snippet,
			Matches: matches,
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

// ResolveID resolves a short ID prefix to the full file path.
// If the prefix is ambiguous or not found, an error is returned.
func (b *BleveIndexer) ResolveID(prefix string) (string, error) {
	idx, err := bleve.Open(b.IndexPath)
	if err != nil {
		return "", fmt.Errorf("open bleve index: %w", err)
	}
	defer idx.Close()

	q := bleve.NewPrefixQuery(prefix)
	q.SetField("id")
	req := bleve.NewSearchRequest(q)
	req.Size = 2
	req.Fields = []string{"path"}
	res, err := idx.Search(req)
	if err != nil {
		return "", err
	}
	if res.Total == 0 {
		return "", fmt.Errorf("id prefix %q not found", prefix)
	}
	if res.Total > 1 {
		return "", fmt.Errorf("id prefix %q is ambiguous (%d matches)", prefix, res.Total)
	}
	path := fieldString(res.Hits[0].Fields["path"])
	return path, nil
}

// extractWindow returns a string of up to size runes centred on centre within runes.
func extractWindow(runes []rune, centre, size int) string {
	if len(runes) <= size {
		return string(runes)
	}
	half := size / 2
	start := centre - half
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > len(runes) {
		end = len(runes)
		start = end - size
		if start < 0 {
			start = 0
		}
	}
	return string(runes[start:end])
}

// buildSnippets finds every occurrence of the matched term in body and returns
// one size-rune window centred on each, joined with " ... ". Overlapping windows
// are merged by skipping. The matched term is extracted from the first Bleve
// fragment that contains a <mark> tag; Bleve only returns one fragment per field
// by default, so we do the multi-occurrence search ourselves.
func buildSnippets(body string, fragments []string, size int) string {
	if size <= 0 {
		size = 150
	}
	bodyRunes := []rune(body)
	if len(bodyRunes) == 0 {
		return ""
	}

	// Extract all unique matched terms from fragment <mark> tags.
	termSet := make(map[string]struct{})
	for _, frag := range fragments {
		remaining := frag
		for {
			start := strings.Index(remaining, "<mark>")
			end := strings.Index(remaining, "</mark>")
			if start < 0 || end <= start {
				break
			}
			term := remaining[start+len("<mark>") : end]
			if term != "" {
				termSet[strings.ToLower(term)] = struct{}{}
			}
			remaining = remaining[end+len("</mark>"):]
		}
	}

	if len(termSet) == 0 {
		return extractWindow(bodyRunes, 0, size)
	}

	// Find every occurrence of every term in the body and collect rune centres.
	lowerBody := strings.ToLower(body)
	var centres []int
	for term := range termSet {
		searchFrom := 0
		for {
			idx := strings.Index(lowerBody[searchFrom:], term)
			if idx < 0 {
				break
			}
			absIdx := searchFrom + idx
			centres = append(centres, len([]rune(body[:absIdx])))
			searchFrom = absIdx + len(term)
		}
	}

	if len(centres) == 0 {
		return extractWindow(bodyRunes, 0, size)
	}

	// Sort centres so windows are emitted in document order.
	for i := 1; i < len(centres); i++ {
		for j := i; j > 0 && centres[j] < centres[j-1]; j-- {
			centres[j], centres[j-1] = centres[j-1], centres[j]
		}
	}

	// Build non-overlapping windows.
	var windows []string
	lastWindowEnd := -1
	for _, centre := range centres {
		half := size / 2
		winStart := centre - half
		if winStart < 0 {
			winStart = 0
		}
		if winStart < lastWindowEnd {
			continue // overlaps with previous window — skip
		}
		winEnd := winStart + size
		if winEnd > len(bodyRunes) {
			winEnd = len(bodyRunes)
		}
		lastWindowEnd = winEnd
		windows = append(windows, extractWindow(bodyRunes, centre, size))
	}

	if len(windows) == 0 {
		return extractWindow(bodyRunes, 0, size)
	}
	return strings.Join(windows, " ... ")
}

// stripTags removes HTML tags from s. Only sequences of the form <letter...>,
// </...>, or <!...> are treated as tags; bare < characters are preserved.
func stripTags(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		ltIdx := strings.IndexByte(s, '<')
		if ltIdx < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:ltIdx])
		rest := s[ltIdx+1:]
		// Only strip if it looks like an HTML tag
		if len(rest) > 0 && (rest[0] == '/' ||
			(rest[0] >= 'a' && rest[0] <= 'z') ||
			(rest[0] >= 'A' && rest[0] <= 'Z') ||
			rest[0] == '!') {
			gtIdx := strings.IndexByte(rest, '>')
			if gtIdx >= 0 {
				s = rest[gtIdx+1:]
				continue
			}
		}
		// Not a tag — emit the '<' and continue
		b.WriteByte('<')
		s = rest
	}
	return b.String()
}

// stemWord lowercases and Porter-stems a single word.
func stemWord(word string) string {
	return string(porterstemmer.StemWithoutLowerCasing([]rune(strings.ToLower(word))))
}

// extractQueryTerms parses a Bleve query string and returns the bare search
// terms with operators (+/-), field prefixes (field:), and quotes stripped.
func extractQueryTerms(queryStr string) []string {
	var terms []string
	for _, token := range strings.Fields(queryStr) {
		token = strings.TrimLeft(token, "+-")
		if i := strings.Index(token, ":"); i >= 0 {
			token = token[i+1:]
		}
		token = strings.Trim(token, "\"")
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			terms = append(terms, token)
		}
	}
	return terms
}

// bodyWords splits body into lowercase alphabetic tokens (simple word tokenizer).
func bodyWords(body string) []string {
	var words []string
	start := -1
	runes := []rune(strings.ToLower(body))
	for i, r := range runes {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlpha {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				words = append(words, string(runes[start:i]))
				start = -1
			}
		}
	}
	if start >= 0 {
		words = append(words, string(runes[start:]))
	}
	return words
}

// countTermMatches counts how many times each query term (or its stemmed
// variants) appears in body as whole words. Keys are the original query terms
// where possible, falling back to the stemmed surface form from fragments.
func countTermMatches(body string, fragments []string, queryStr string) map[string]int {
	// Build stem → canonical key map, preferring query terms as keys.
	stemToKey := make(map[string]string)

	for _, qt := range extractQueryTerms(queryStr) {
		s := stemWord(qt)
		if _, exists := stemToKey[s]; !exists {
			stemToKey[s] = qt
		}
	}
	// Supplement with surface forms from <mark> tags (catches terms not in the
	// simple query parse, e.g. field queries whose values appear highlighted).
	for _, frag := range fragments {
		remaining := frag
		for {
			start := strings.Index(remaining, "<mark>")
			end := strings.Index(remaining, "</mark>")
			if start < 0 || end <= start {
				break
			}
			term := strings.ToLower(remaining[start+len("<mark>") : end])
			if term != "" {
				s := stemWord(term)
				if _, exists := stemToKey[s]; !exists {
					stemToKey[s] = term
				}
			}
			remaining = remaining[end+len("</mark>"):]
		}
	}

	if len(stemToKey) == 0 {
		return nil
	}

	// Count whole-word matches in the body using stemmed comparison.
	counts := make(map[string]int)
	for _, word := range bodyWords(body) {
		s := stemWord(word)
		if key, ok := stemToKey[s]; ok {
			counts[key]++
		}
	}

	if len(counts) == 0 {
		return nil
	}
	return counts
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
	// Count actual documents (excluding internal metadata docs).
	// Probe uses size=0, so res.Hits is empty — use res.Total directly.
	// Subtract 1 if the metadata document exists in the index.
	totalMatches := int(res.Total)
	if metaDoc, _ := idx.Document("__meta__prefix_len"); metaDoc != nil {
		totalMatches--
		if totalMatches < 0 {
			totalMatches = 0
		}
	}

	return ProbeResult{
		Query:        queryStr,
		TotalMatches: totalMatches,
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
