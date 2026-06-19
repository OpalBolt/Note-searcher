package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLiteIndexer is the SQLite-backed search index.
type SQLiteIndexer struct {
	DBPath string
}

func NewSQLiteIndexer(dbPath string) *SQLiteIndexer {
	return &SQLiteIndexer{DBPath: dbPath}
}

// openRW opens the database for read-write (used by Build).
func (s *SQLiteIndexer) openRW() (*sql.DB, error) {
	return sql.Open("sqlite", s.DBPath)
}

// openRO opens the database read-only (used by all query commands).
func (s *SQLiteIndexer) openRO() (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+s.DBPath+"?mode=ro")
}

// docID computes the sha256 hex ID for a relative path.
func docID(relPath string) string {
	h := sha256.Sum256([]byte(relPath))
	return hex.EncodeToString(h[:])
}

// minUniquePrefixLen returns the smallest n ≥ 7 at which no two IDs share an n-char prefix.
func minUniquePrefixLen(ids []string) int {
	for n := 7; n <= 64; n++ {
		seen := make(map[string]bool, len(ids))
		collision := false
		for _, id := range ids {
			if len(id) < n {
				continue
			}
			pfx := id[:n]
			if seen[pfx] {
				collision = true
				break
			}
			seen[pfx] = true
		}
		if !collision {
			return n
		}
	}
	return 64
}

// readPrefixLen reads the stored minimum unique ID prefix length from the DB.
// Falls back to 7 if not set.
func readPrefixLen(db *sql.DB) int {
	var v string
	if err := db.QueryRow(`SELECT value FROM _meta WHERE key = 'id_prefix_len'`).Scan(&v); err != nil {
		return 7
	}
	n := 7
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 7
	}
	return n
}

// ReadNotesDir returns the absolute notes directory stored during Build.
// Falls back to empty string if not available.
func (s *SQLiteIndexer) ReadNotesDir() string {
	db, err := s.openRO()
	if err != nil {
		return ""
	}
	defer db.Close()
	var v string
	db.QueryRow(`SELECT value FROM _meta WHERE key = 'notes_dir'`).Scan(&v)
	return v
}

// Build walks notesDir, parses frontmatter, and populates the SQLite database.
func (s *SQLiteIndexer) Build(notesDir string) (IndexStats, error) {
	if err := os.RemoveAll(s.DBPath); err != nil {
		return IndexStats{}, fmt.Errorf("remove old db: %w", err)
	}

	db, err := s.openRW()
	if err != nil {
		return IndexStats{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return IndexStats{}, err
	}

	schema := `
CREATE TABLE docs (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL UNIQUE,
  title TEXT,
  created TEXT, updated TEXT,
  status TEXT, confidence TEXT, type TEXT,
  scope TEXT, project TEXT,
  source_agent TEXT, source_artifact TEXT,
  review_by TEXT,
  requires_human_review INTEGER DEFAULT 0,
  superseded_by TEXT,
  is_superseded INTEGER DEFAULT 0,
  line_count INTEGER DEFAULT 0,
  chars INTEGER DEFAULT 0
);
CREATE TABLE docs_body (
  id TEXT PRIMARY KEY REFERENCES docs(id),
  body TEXT NOT NULL
);
CREATE TABLE doc_tags   (id TEXT, tag    TEXT NOT NULL);
CREATE TABLE doc_domains(id TEXT, domain TEXT NOT NULL);
CREATE VIRTUAL TABLE docs_fts USING fts5(
  id UNINDEXED, title, body,
  tokenize='porter ascii'
);
CREATE INDEX idx_doc_tags_id    ON doc_tags(id);
CREATE INDEX idx_doc_domains_id ON doc_domains(id);
CREATE TABLE _meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`
	if _, err := db.Exec(schema); err != nil {
		return IndexStats{}, fmt.Errorf("create schema: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return IndexStats{}, err
	}

	stmtDoc, err := tx.Prepare(`INSERT INTO docs
		(id,path,title,created,updated,status,confidence,type,scope,project,
		 source_agent,source_artifact,review_by,requires_human_review,
		 superseded_by,is_superseded,line_count,chars)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return IndexStats{}, fmt.Errorf("prepare docs stmt: %w", err)
	}
	stmtBody, err := tx.Prepare(`INSERT INTO docs_body (id,body) VALUES (?,?)`)
	if err != nil {
		tx.Rollback()
		return IndexStats{}, fmt.Errorf("prepare body stmt: %w", err)
	}
	stmtTag, err := tx.Prepare(`INSERT INTO doc_tags   (id,tag)    VALUES (?,?)`)
	if err != nil {
		tx.Rollback()
		return IndexStats{}, fmt.Errorf("prepare tag stmt: %w", err)
	}
	stmtDom, err := tx.Prepare(`INSERT INTO doc_domains(id,domain) VALUES (?,?)`)
	if err != nil {
		tx.Rollback()
		return IndexStats{}, fmt.Errorf("prepare domain stmt: %w", err)
	}
	stmtFts, err := tx.Prepare(`INSERT INTO docs_fts   (id,title,body) VALUES (?,?,?)`)
	if err != nil {
		tx.Rollback()
		return IndexStats{}, fmt.Errorf("prepare fts stmt: %w", err)
	}

	var count int
	var allIDs []string
	err = filepath.Walk(notesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, _ := filepath.Rel(notesDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, body, err := ParseFrontmatter(data)
		if err != nil {
			return nil
		}
		id := docID(rel)
		isSuperseded := 0
		if meta.SupersededBy != "" {
			isSuperseded = 1
		}
		reqReview := 0
		if meta.RequiresHumanReview {
			reqReview = 1
		}
		chars := len([]rune(body))
		lines := strings.Count(body, "\n") + 1

		// Convert date fields to string format
		created := formatDateToString(meta.Created)
		updated := formatDateToString(meta.Updated)
		reviewBy := formatDateToString(meta.ReviewBy)

		// Handle Domain field
		var domain []string
		if meta.Domain != nil {
			if ds, ok := meta.Domain.([]string); ok {
				domain = ds
			}
		}
		if domain == nil {
			domain = []string{}
		}

		if _, err := stmtDoc.Exec(id, rel,
			meta.Title, created, updated,
			meta.Status, meta.Confidence, meta.Type,
			meta.Scope, meta.Project,
			meta.SourceAgent, meta.SourceArtifact, reviewBy,
			reqReview, meta.SupersededBy, isSuperseded, lines, chars,
		); err != nil {
			return nil
		}
		stmtBody.Exec(id, body)
		for _, tag := range meta.Tags {
			stmtTag.Exec(id, tag)
		}
		for _, dom := range domain {
			stmtDom.Exec(id, dom)
		}
		stmtFts.Exec(id, meta.Title, body)
		allIDs = append(allIDs, id)
		count++
		return nil
	})
	if err != nil {
		tx.Rollback()
		return IndexStats{}, err
	}
	if err := tx.Commit(); err != nil {
		return IndexStats{}, err
	}

	// Compute and store minimum unique ID prefix length
	pfxLen := minUniquePrefixLen(allIDs)
	if _, err := db.Exec(`INSERT INTO _meta (key, value) VALUES ('id_prefix_len', ?)`,
		fmt.Sprintf("%d", pfxLen)); err != nil {
		return IndexStats{}, fmt.Errorf("store prefix len: %w", err)
	}
	// Store the absolute notes dir so query commands can resolve paths without --notes-dir
	absNotesDir, _ := filepath.Abs(notesDir)
	if _, err := db.Exec(`INSERT INTO _meta (key, value) VALUES ('notes_dir', ?)`, absNotesDir); err != nil {
		return IndexStats{}, fmt.Errorf("store notes_dir: %w", err)
	}

	fi, _ := os.Stat(s.DBPath)
	var sz int64
	if fi != nil {
		sz = fi.Size()
	}
	return IndexStats{FileCount: count, IndexBytes: sz}, nil
}

// formatDateToString converts a date field to "2006-01-02" format or empty string
func formatDateToString(field interface{}) string {
	if field == nil {
		return ""
	}
	if s, ok := field.(string); ok {
		return s
	}
	return ""
}

// buildWhere constructs the WHERE clause and args for probe/search filters.
// baseJoin is extra JOIN clauses needed (e.g. tag/domain junction tables).
// Returns (whereClause, joinClause, args).
func buildWhere(opts SearchOptions) (string, string, []interface{}) {
	var clauses []string
	var args []interface{}
	var joins []string

	if !opts.Superseded {
		clauses = append(clauses, "d.is_superseded = 0")
	}
	if opts.Status != "" {
		clauses = append(clauses, "d.status = ?")
		args = append(args, opts.Status)
	}
	if opts.Confidence != "" {
		clauses = append(clauses, "d.confidence = ?")
		args = append(args, opts.Confidence)
	}
	if opts.Type != "" {
		clauses = append(clauses, "d.type = ?")
		args = append(args, opts.Type)
	}
	if opts.Scope != "" {
		clauses = append(clauses, "d.scope = ?")
		args = append(args, opts.Scope)
	}
	if opts.Project != "" {
		clauses = append(clauses, "d.project = ?")
		args = append(args, opts.Project)
	}
	if opts.Tag != "" {
		joins = append(joins, "JOIN doc_tags dt ON dt.id = d.id")
		clauses = append(clauses, "dt.tag = ?")
		args = append(args, opts.Tag)
	}
	if opts.Domain != "" {
		joins = append(joins, "JOIN doc_domains dd ON dd.id = d.id")
		clauses = append(clauses, "dd.domain = ?")
		args = append(args, opts.Domain)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	return where, strings.Join(joins, " "), args
}

// buildWhereProbe is the same but takes ProbeOptions.
func buildWhereProbe(opts ProbeOptions) (string, string, []interface{}) {
	return buildWhere(SearchOptions{
		Status: opts.Status, Confidence: opts.Confidence, Type: opts.Type,
		Scope: opts.Scope, Project: opts.Project, Tag: opts.Tag, Domain: opts.Domain,
		Superseded: opts.Superseded,
	})
}

// Search returns ranked metadata results, optionally with body snippets.
func (s *SQLiteIndexer) Search(query string, opts SearchOptions) (SearchResponse, error) {
	db, err := s.openRO()
	if err != nil {
		return SearchResponse{}, err
	}
	defer db.Close()

	where, joins, args := buildWhere(opts)

	var q string

	snipChars := opts.SnippetSize
	if snipChars <= 0 {
		snipChars = 150
	}

	if query != "" {
		// FTS5 join path
		matchCond := "f.docs_fts MATCH ?"
		if where != "" {
			where = "WHERE " + matchCond + " AND f.id = d.id AND " + strings.TrimPrefix(where, "WHERE ")
		} else {
			where = "WHERE " + matchCond + " AND f.id = d.id"
		}

		if opts.Snippets {
			// Fetch raw body; extract context windows in Go (avoids FTS5 auxiliary fn issues)
			q = fmt.Sprintf(`
SELECT d.id, d.path, d.title, d.status, d.confidence, d.type, d.scope, d.project, d.chars,
       COALESCE(b.body, '') AS body,
       f.rank
FROM docs_fts f
JOIN docs d ON d.id = f.id
LEFT JOIN docs_body b ON b.id = f.id
%s
%s
ORDER BY f.rank
`, joins, where)
		} else {
			q = fmt.Sprintf(`
SELECT d.id, d.path, d.title, d.status, d.confidence, d.type, d.scope, d.project, d.chars,
       '' AS body, f.rank
FROM docs_fts f
JOIN docs d ON d.id = f.id
%s
%s
ORDER BY f.rank
`, joins, where)
		}

		// FTS match term goes first in args
		args = append([]interface{}{query}, args...)
	} else {
		// No FTS — metadata scan, optional body excerpt
		if where == "" {
			where = "WHERE 1=1"
		}
		if opts.Snippets {
			// No query — return start of document as the snippet
			q = fmt.Sprintf(`
SELECT d.id, d.path, d.title, d.status, d.confidence, d.type, d.scope, d.project, d.chars,
       COALESCE(SUBSTR(b.body, 1, %d), '') AS snippet, 0.0 AS rank
FROM docs d
LEFT JOIN docs_body b ON b.id = d.id
%s
%s
ORDER BY d.path
`, snipChars, joins, where)
		} else {
			q = fmt.Sprintf(`
SELECT d.id, d.path, d.title, d.status, d.confidence, d.type, d.scope, d.project, d.chars,
       '' AS snippet, 0.0 AS rank
FROM docs d
%s
%s
ORDER BY d.path
`, joins, where)
		}
	}

	// Count total matches before applying LIMIT
	var total int
	if opts.Limit > 0 {
		countQ := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", q)
		db.QueryRow(countQ, args...).Scan(&total)
		q += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	pfxLen := readPrefixLen(db)
	for rows.Next() {
		var r SearchResult
		var fullID, rawBody string
		var score float64
		if err := rows.Scan(&fullID, &r.Path, &r.Title, &r.Status, &r.Confidence, &r.Type, &r.Scope, &r.Project, &r.Chars, &rawBody, &score); err != nil {
			continue
		}
		r.ID = fullID[:pfxLen]
		// FTS5 rank is negative BM25 (more negative = better match).
		// Negate so higher score = more relevant. Zero when no query.
		if score != 0 {
			r.Score = -score
		}
		if query != "" && opts.Snippets {
			// Extract context windows around query terms from raw body
			r.Snippet = extractSnippetWindows(rawBody, query, snipChars)
		} else {
			// No-query snippets: rawBody is already SUBSTR'd by SQL
			r.Snippet = rawBody
		}
		r.Tags = loadStringSlice(db, "SELECT tag FROM doc_tags WHERE id = ?", fullID)
		r.Domain = loadStringSlice(db, "SELECT domain FROM doc_domains WHERE id = ?", fullID)
		results = append(results, r)
	}

	if opts.Limit == 0 {
		total = len(results)
	}

	return SearchResponse{
		Total:   total,
		Shown:   len(results),
		Query:   query,
		Results: results,
	}, nil
}



// extractSnippetWindows finds every occurrence of each word in query within body
// (case-insensitive) and returns contextChars of text around each hit,
// with overlapping windows merged and joined by " \u2026 ".
func extractSnippetWindows(body, query string, contextChars int) string {
	if body == "" || query == "" {
		return ""
	}

	lowerBody := strings.ToLower(body)
	words := strings.Fields(strings.ToLower(query))

	type window struct{ s, e int }
	var windows []window

	for _, word := range words {
		if len(word) < 2 {
			continue
		}
		pos := 0
		for {
			idx := strings.Index(lowerBody[pos:], word)
			if idx < 0 {
				break
			}
			idx += pos
			wS := idx - contextChars
			if wS < 0 {
				wS = 0
			}
			wE := idx + len(word) + contextChars
			if wE > len(body) {
				wE = len(body)
			}
			windows = append(windows, window{wS, wE})
			pos = idx + len(word)
		}
	}

	if len(windows) == 0 {
		return ""
	}

	// Sort by start position before merging
	sort.Slice(windows, func(i, j int) bool { return windows[i].s < windows[j].s })

	// Merge overlapping windows
	merged := []window{windows[0]}
	for _, w := range windows[1:] {
		last := &merged[len(merged)-1]
		if w.s <= last.e {
			if w.e > last.e {
				last.e = w.e
			}
		} else {
			merged = append(merged, w)
		}
	}

	// Extract text and join
	var parts []string
	for _, w := range merged {
		chunk := strings.TrimSpace(body[w.s:w.e])
		if chunk != "" {
			parts = append(parts, chunk)
		}
	}
	return strings.Join(parts, " \u2026 ")
}


// GetBody returns the stored body text for a document by relative path.
func (s *SQLiteIndexer) GetBody(relPath string) (string, error) {
	db, err := s.openRO()
	if err != nil {
		return "", err
	}
	defer db.Close()
	var body string
	err = db.QueryRow(
		`SELECT b.body FROM docs_body b JOIN docs d ON d.id = b.id WHERE d.path = ?`,
		relPath,
	).Scan(&body)
	if err != nil {
		return "", err
	}
	return body, nil
}

// loadStringSlice is a helper to load a []string from a single-column query.
func loadStringSlice(db *sql.DB, q, id string) []string {
	rows, err := db.Query(q, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

// Probe returns facet distribution for the matching result set.
func (s *SQLiteIndexer) Probe(query string, opts ProbeOptions) (ProbeResult, error) {
	db, err := s.openRO()
	if err != nil {
		return ProbeResult{}, err
	}
	defer db.Close()

	where, joins, args := buildWhereProbe(opts)

	// Build the candidate ID set
	var idQuery string
	var idArgs []interface{}

	if query != "" {
		idQuery = fmt.Sprintf(`
SELECT d.id FROM docs_fts f
JOIN docs d ON d.id = f.id
%s
WHERE f.docs_fts MATCH ? %s
`, joins, func() string {
			if where != "" && where != "WHERE" {
				return " AND " + strings.TrimPrefix(where, "WHERE ")
			}
			return ""
		}())
		idArgs = append([]interface{}{query}, args...)
	} else {
		idQuery = fmt.Sprintf(`SELECT d.id FROM docs d %s %s`, joins, where)
		idArgs = args
}

	// Wrap as CTE for facet queries
	cte := "WITH candidates AS (" + idQuery + ")"

	// Optional per-facet value cap, ordered by count descending
	facetLimit := ""
	if opts.Limit > 0 {
		facetLimit = fmt.Sprintf(" ORDER BY COUNT(*) DESC LIMIT %d", opts.Limit)
	}

	facets := make(map[string]FacetCounts)

	for _, field := range []string{"status", "type", "confidence", "project"} {
		fc := FacetCounts{}
		rows, err := db.Query(fmt.Sprintf(
			`%s SELECT d.%s, COUNT(*) FROM docs d JOIN candidates c ON c.id = d.id
			 WHERE d.%s IS NOT NULL AND d.%s != ''
			 GROUP BY d.%s%s`, cte, field, field, field, field, facetLimit),
			idArgs...)
		if err == nil {
			for rows.Next() {
				var k string
				var n int
				if rows.Scan(&k, &n) == nil {
					fc[k] = n
				}
			}
			rows.Close()
		}
		if len(fc) > 0 {
			facets[field] = fc
		}
	}

	// Tags facet via junction table
	tagFC := FacetCounts{}
	rows, err := db.Query(fmt.Sprintf(
		`%s SELECT t.tag, COUNT(*) FROM doc_tags t JOIN candidates c ON c.id = t.id GROUP BY t.tag%s`,
		cte, facetLimit), idArgs...)
	if err == nil {
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				tagFC[k] = n
			}
		}
		rows.Close()
	}
	if len(tagFC) > 0 {
		facets["tags"] = tagFC
	}

	// Total count
	var total int
	db.QueryRow(fmt.Sprintf(`%s SELECT COUNT(*) FROM candidates`, cte), idArgs...).Scan(&total)

	return ProbeResult{
		Query:        query,
		TotalMatches: total,
		Facets:       facets,
	}, nil
}

// Get retrieves a full document by short ID prefix or relative path.
func (s *SQLiteIndexer) Get(idOrPath string) (GetResult, error) {
	db, err := s.openRO()
	if err != nil {
		return GetResult{}, err
	}
	defer db.Close()

	var id, path, title, status, confidence, docType, scope, project string
	var created, updated, sourceAgent, sourceArtifact, reviewBy, supersededBy string
	var reqReview, isSuperseded, lineCount, chars int

	// Try exact path first, then ID prefix
	row := db.QueryRow(`SELECT id,path,title,created,updated,status,confidence,type,scope,project,
		source_agent,source_artifact,review_by,requires_human_review,superseded_by,is_superseded,line_count,chars
		FROM docs WHERE path = ?`, idOrPath)
	err = row.Scan(&id, &path, &title, &created, &updated, &status, &confidence, &docType, &scope, &project,
		&sourceAgent, &sourceArtifact, &reviewBy, &reqReview, &supersededBy, &isSuperseded, &lineCount, &chars)
	if err == sql.ErrNoRows {
		// Try ID prefix
		rows, qErr := db.Query(`SELECT id,path,title,created,updated,status,confidence,type,scope,project,
			source_agent,source_artifact,review_by,requires_human_review,superseded_by,is_superseded,line_count,chars
			FROM docs WHERE id LIKE ?`, idOrPath+"%")
		if qErr != nil {
			return GetResult{}, qErr
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			if found {
				return GetResult{}, fmt.Errorf("ambiguous ID prefix %q: multiple documents match", idOrPath)
			}
			rows.Scan(&id, &path, &title, &created, &updated, &status, &confidence, &docType, &scope, &project,
				&sourceAgent, &sourceArtifact, &reviewBy, &reqReview, &supersededBy, &isSuperseded, &lineCount, &chars)
			found = true
		}
		if !found {
			return GetResult{}, fmt.Errorf("document not found: %s", idOrPath)
		}
	} else if err != nil {
		return GetResult{}, err
	}

	tags := loadStringSlice(db, "SELECT tag FROM doc_tags WHERE id = ?", id)
	domains := loadStringSlice(db, "SELECT domain FROM doc_domains WHERE id = ?", id)

	var body string
	db.QueryRow("SELECT body FROM docs_body WHERE id = ?", id).Scan(&body)

	meta := DocumentMeta{
		Path:                path,
		Title:               title,
		Created:             created,
		Updated:             updated,
		Status:              status,
		Confidence:          confidence,
		Type:                docType,
		Scope:               scope,
		Project:             project,
		SourceAgent:         sourceAgent,
		SourceArtifact:      sourceArtifact,
		ReviewBy:            reviewBy,
		RequiresHumanReview: reqReview == 1,
		SupersededBy:        supersededBy,
		Tags:                tags,
		Domain:              domains,
		LineCount:           lineCount,
	}

	return GetResult{ID: id, Path: path, Meta: meta, Body: body}, nil
}

// Context returns schema information, sample field values, and a workflow guide.
func (s *SQLiteIndexer) Context() (ContextResult, error) {
	db, err := s.openRO()
	if err != nil {
		return ContextResult{}, err
	}
	defer db.Close()

	samples := make(map[string][]string)
	for _, field := range []string{"status", "type", "scope", "project"} {
		rows, err := db.Query(fmt.Sprintf(
			`SELECT DISTINCT %s FROM docs WHERE %s IS NOT NULL AND %s != '' ORDER BY %s LIMIT 20`,
			field, field, field, field))
		if err != nil {
			continue
		}
		var vals []string
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				vals = append(vals, v)
			}
		}
		rows.Close()
		if len(vals) > 0 {
			samples[field] = vals
		}
	}
	// tags
	tagRows, _ := db.Query(`SELECT DISTINCT tag FROM doc_tags ORDER BY tag LIMIT 30`)
	var tags []string
	if tagRows != nil {
		for tagRows.Next() {
			var t string
			if tagRows.Scan(&t) == nil {
				tags = append(tags, t)
			}
		}
		tagRows.Close()
	}
	if len(tags) > 0 {
		samples["tags"] = tags
	}
	// domains
	domRows, _ := db.Query(`SELECT DISTINCT domain FROM doc_domains ORDER BY domain LIMIT 30`)
	var doms []string
	if domRows != nil {
		for domRows.Next() {
			var d string
			if domRows.Scan(&d) == nil {
				doms = append(doms, d)
			}
		}
		domRows.Close()
	}
	if len(doms) > 0 {
		samples["domain"] = doms
	}

	schema := map[string]interface{}{
		"tables": []map[string]interface{}{
			{"name": "docs", "description": "one row per note, all frontmatter fields as columns"},
			{"name": "docs_body", "description": "body text stored separately; id FK to docs"},
			{"name": "doc_tags", "description": "junction table for multi-value tags; columns: id, tag"},
			{"name": "doc_domains", "description": "junction table for multi-value domain; columns: id, domain"},
			{"name": "docs_fts", "description": "FTS5 virtual table over title+body with porter stemmer; columns: id, title, body"},
		},
		"docs_columns": []string{
			"id", "path", "title", "created", "updated",
			"status", "type", "scope", "project",
			"source_agent", "source_artifact", "review_by",
			"requires_human_review", "superseded_by", "is_superseded",
			"line_count", "chars",
		},
	}

	workflow := "Step 1: Call context once to bootstrap — learn the schema, available field values, and how commands compose." +
		" Step 2: Use probe [query] to check facet distributions before committing to a search. Probe returns counts only, no body text." +
		" Step 3: Use search [query] to get ranked results. Default output is id + title only (max 50 results)." +
		" Step 4: Narrow with filter flags (--status, --type, --tag etc.) and re-probe or re-search until the result set is useful." +
		" Step 5: Add --fields to include extra metadata columns: status, domain, tags, chars, score, confidence, type, scope, project." +
		" Step 6: Add --headings to include H1-H6 heading structure for each result." +
		" Step 7: Add --snippet-size N (chars) to include body excerpts around every hit. Implies --fields snippet." +
		" Step 8: Use get <id> with the short ID from search results to retrieve the full document. Never use paths — always use IDs." +
		" Step 9: Use get <id> --titles-only, --section, or --section-search to navigate document structure without reading the full body."

	commands := []map[string]interface{}{
		{
			"name":        "context",
			"description": "Bootstrap: returns schema, sample field values, workflow, and command reference. Call once before any session.",
		},
		{
			"name":        "probe [query]",
			"description": "Facet counts over matching documents. No body text returned. Use before search to understand the distribution.",
			"filter_flags": []string{"--status", "--confidence", "--type", "--scope", "--project", "--tag", "--domain", "--superseded"},
			"other_flags":  map[string]string{
				"--limit N": "return only the top N values per facet, ordered by count descending",
				"--pretty":  "pretty-print JSON output",
			},
		},
		{
			"name":        "search [query]",
			"description": "Ranked full-text + metadata search. Default output: id + title only, max 50 results. IDs are short unique prefixes — use them with get.",
			"filter_flags": []string{"--status", "--confidence", "--type", "--scope", "--project", "--tag", "--domain", "--superseded"},
			"output_flags": map[string]string{
				"--fields status,...": "add scalar columns: status,domain,tags,chars,type,scope,project",
				"--fields score":      "add BM25 relevance score (higher = more relevant; only meaningful when a query is given)",
				"--headings":         "add H1-H6 heading list to each result",
				"--snippets":         "add body excerpts (no query = start of file)",
				"--snippet-size N":   "chars of context around each hit; implies --snippets and --fields snippet (default 150)",
			},
			"other_flags": []string{"--limit", "--pretty"},
		},
		{
			"name":        "get <id>",
			"description": "Retrieve a document by short ID from search results. Always use the ID — never the path.",
			"flags": map[string]string{
				"--full":           "frontmatter metadata + body",
				"--metadata-only":  "frontmatter fields only, no body",
				"--titles-only":    "H1-H6 heading list only",
				"--section <id>":   "extract a single section by heading ID (e.g. h2)",
				"--section-search": "extract all sections containing a term",
			},
		},
	}

	return ContextResult{
		Schema:   schema,
		Samples:  samples,
		Workflow: workflow,
		Commands: commands,
	}, nil
}

// SQL executes a read-only SELECT query and returns rows as a slice of maps.
func (s *SQLiteIndexer) SQL(query string) ([]map[string]interface{}, error) {
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") {
		return nil, fmt.Errorf("only SELECT queries are allowed")
	}
	// Reject multi-statement queries (semicolons in value position are caught by the driver,
	// but a bare second statement like '; DROP TABLE' could slip past simple prefix checks).
	if strings.Contains(trimmed, ";") {
		return nil, fmt.Errorf("only single SELECT statements are allowed (no semicolons)")
	}

	db, err := s.openRO()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sql: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0) // never nil; encodes as [] not null
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		out = append(out, row)
	}
	return out, nil
}
