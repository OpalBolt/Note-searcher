package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpalBolt/note-searcher/internal/config"
	"github.com/OpalBolt/note-searcher/internal/get"
	"github.com/OpalBolt/note-searcher/internal/index"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "note-searcher",
	Short: "Search and index your notes",
	Long:  "note-searcher indexes markdown notes with SQLite FTS5 and provides fast full-text and metadata search",
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build the SQLite FTS5 search index from the notes directory",
	RunE:  runIndex,
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search notes using full-text and metadata filters",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSearch,
}

var probeCmd = &cobra.Command{
	Use:   "probe [query]",
	Short: "Probe the search space: returns facet counts for the matching result set",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProbe,
}

var getCmd = &cobra.Command{
	Use:   "get <path>...",
	Short: "Retrieve note content by file path",
	Args:  cobra.ArbitraryArgs,
	RunE:  runGet,
}

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Output schema, sample field values, and workflow guide for LLM bootstrapping",
	RunE:  runContext,
}

var sqlCmd = &cobra.Command{
	Use:   "sql <query>",
	Short: "Execute a read-only SELECT query against the notes database",
	Args:  cobra.ExactArgs(1),
	RunE:  runSQL,
}

func init() {
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(probeCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(sqlCmd)
	rootCmd.AddCommand(guideCmd)

	// Persistent flags available to all subcommands
	rootCmd.PersistentFlags().String("notes-dir", "./notes", "directory containing markdown notes")
	rootCmd.PersistentFlags().String("index-path", "./notes.db", "path to the SQLite database")

	// search-specific flags
	searchCmd.Flags().String("status", "", "Filter by status")
	searchCmd.Flags().String("confidence", "", "Filter by confidence")
	searchCmd.Flags().String("type", "", "Filter by type")
	searchCmd.Flags().String("scope", "", "Filter by scope")
	searchCmd.Flags().String("project", "", "Filter by project")
	searchCmd.Flags().String("tag", "", "Filter by tag")
	searchCmd.Flags().String("domain", "", "Filter by domain")
	searchCmd.Flags().Bool("superseded", false, "Include superseded documents")
	searchCmd.Flags().Int("limit", 50, "Return top N results (0 = no limit)")
	searchCmd.Flags().String("fields", "", "Extra fields to include beyond id+title. Comma-separated: path,status,domain,tags,chars,snippet,score,confidence,type,scope,project")
	searchCmd.Flags().Bool("snippets", false, "Include body excerpts in results")
	searchCmd.Flags().Int("snippet-size", 0, "Characters of context around each hit; implies --snippets and --fields snippet (default 150 when set)")
	searchCmd.Flags().Bool("headings", false, "Include heading structure (H1-H6) in each result")
	searchCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")

	// probe-specific flags
	probeCmd.Flags().String("status", "", "Filter by status")
	probeCmd.Flags().String("confidence", "", "Filter by confidence")
	probeCmd.Flags().String("type", "", "Filter by type")
	probeCmd.Flags().String("scope", "", "Filter by scope")
	probeCmd.Flags().String("project", "", "Filter by project")
	probeCmd.Flags().String("tag", "", "Filter by tag")
	probeCmd.Flags().String("domain", "", "Filter by domain")
	probeCmd.Flags().Bool("superseded", false, "Include superseded documents")
	probeCmd.Flags().Int("limit", 0, "Max values to return per facet, ordered by count desc (0 = no limit)")
	probeCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")

	// get-specific flags
	getCmd.Flags().Bool("full", false, "Return frontmatter + body")
	getCmd.Flags().Bool("metadata-only", false, "Return frontmatter fields only (no body)")
	getCmd.Flags().Bool("pretty", false, "Pretty-print JSON output")
	getCmd.Flags().String("format", "json", "Output format: json or text")
	getCmd.Flags().Bool("titles-only", false, "Return heading structure only (H1-H6 with sequential IDs)")
	getCmd.Flags().String("section", "", "Return content of section with given ID (e.g. h3)")
	getCmd.Flags().String("section-search", "", "Return all sections containing the given term (case-insensitive)")
	getCmd.Flags().StringSlice("by-path", nil, "Retrieve document(s) by full file path (repeatable)")

	// context and sql flags
	contextCmd.Flags().Bool("pretty", false, "Pretty-print JSON output")
	sqlCmd.Flags().Bool("pretty", false, "Pretty-print JSON output")

	// Bind persistent flags to viper
	_ = viper.BindPFlag("notes-dir", rootCmd.PersistentFlags().Lookup("notes-dir"))
	_ = viper.BindPFlag("index-path", rootCmd.PersistentFlags().Lookup("index-path"))

	setupViper()
}

func setupViper() {
	viper.SetDefault("notes-dir", "./notes")
	viper.SetDefault("index-path", "./notes.db")

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	configDir, err := os.UserConfigDir()
	if err != nil {
		if homeDir, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(homeDir, ".config")
		}
	}
	if configDir != "" {
		viper.AddConfigPath(filepath.Join(configDir, "note-searcher"))
	}

	_ = viper.ReadInConfig()
}

func runIndex(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	indexer := index.NewSQLiteIndexer(cfg.IndexPath)
	stats, err := indexer.Build(cfg.NotesDir)
	if err != nil {
		return fmt.Errorf("build index: %w", err)
	}

	fmt.Printf("Indexed %d files, index size %d KB\n", stats.FileCount, stats.IndexBytes/1024)
	return nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var queryStr string
	if len(args) > 0 {
		queryStr = args[0]
	}

	// Get flags
	snippets, _ := cmd.Flags().GetBool("snippets")
	status, _ := cmd.Flags().GetString("status")
	confidence, _ := cmd.Flags().GetString("confidence")
	typeFilter, _ := cmd.Flags().GetString("type")
	scope, _ := cmd.Flags().GetString("scope")
	project, _ := cmd.Flags().GetString("project")
	tag, _ := cmd.Flags().GetString("tag")
	domain, _ := cmd.Flags().GetString("domain")
	superseded, _ := cmd.Flags().GetBool("superseded")
	limit, _ := cmd.Flags().GetInt("limit")
	snippetSize, _ := cmd.Flags().GetInt("snippet-size")
	headings, _ := cmd.Flags().GetBool("headings")
	pretty, _ := cmd.Flags().GetBool("pretty")
	fieldsRaw, _ := cmd.Flags().GetString("fields")

	// Parse --fields into a set
	fieldsSet := make(map[string]bool)
	validFields := map[string]bool{
		"path": true, "status": true, "domain": true, "tags": true,
		"chars": true, "snippet": true, "score": true,
		"confidence": true, "type": true, "scope": true, "project": true,
	}
	if fieldsRaw != "" {
		for _, f := range strings.Split(fieldsRaw, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if !validFields[f] {
				return fmt.Errorf("unknown field %q: valid fields are path,status,domain,tags,chars,snippet,score,confidence,type,scope,project", f)
			}
			fieldsSet[f] = true
		}
	}
	// snippet in --fields implies --snippets
	if fieldsSet["snippet"] {
		snippets = true
	}
	// --snippet-size implies --snippets and --fields snippet
	if snippetSize > 0 {
		snippets = true
		fieldsSet["snippet"] = true
	}

	opts := index.SearchOptions{
		Status:      status,
		Confidence:  confidence,
		Type:        typeFilter,
		Scope:       scope,
		Project:     project,
		Tag:         tag,
		Domain:      domain,
		Superseded:  superseded,
		Limit:       limit,
		Snippets:    snippets,
		SnippetSize: snippetSize,
	}

	indexer := index.NewSQLiteIndexer(cfg.IndexPath)
	resp, err := indexer.Search(queryStr, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	// Load headings for all results in one pass if requested
	var headingsByPath map[string][]get.Heading
	if headings && len(resp.Results) > 0 {
		headingsByPath = make(map[string][]get.Heading)
		for _, r := range resp.Results {
			body, err := indexer.GetBody(r.Path)
			if err == nil && body != "" {
				headingsByPath[r.Path] = get.ParseHeadings(body)
			}
		}
	}

	// Build filtered results: default = id + title only
	type filteredResult = map[string]interface{}
	var filtered []filteredResult
	for _, r := range resp.Results {
		m := filteredResult{"id": r.ID, "title": r.Title}
		if fieldsSet["path"] {
			m["path"] = r.Path
		}
		if fieldsSet["status"] {
			m["status"] = r.Status
		}
		if fieldsSet["domain"] {
			m["domain"] = r.Domain
		}
		if fieldsSet["tags"] {
			m["tags"] = r.Tags
		}
		if fieldsSet["chars"] {
			m["chars"] = r.Chars
		}
		if fieldsSet["snippet"] {
			m["snippet"] = r.Snippet
		}
		if fieldsSet["score"] {
			m["score"] = r.Score
		}
		if fieldsSet["confidence"] {
			m["confidence"] = r.Confidence
		}
		if fieldsSet["type"] {
			m["type"] = r.Type
		}
		if fieldsSet["scope"] {
			m["scope"] = r.Scope
		}
		if fieldsSet["project"] {
			m["project"] = r.Project
		}
		if headings {
			if hs, ok := headingsByPath[r.Path]; ok {
				m["headings"] = hs
			} else {
				m["headings"] = []get.Heading{}
			}
		}
		filtered = append(filtered, m)
	}

	out := map[string]interface{}{
		"total":   resp.Total,
		"shown":   resp.Shown,
		"query":   resp.Query,
		"results": filtered,
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(out)
}

func runProbe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var queryStr string
	if len(args) > 0 {
		queryStr = args[0]
	}

	// Get flags
	status, _ := cmd.Flags().GetString("status")
	confidence, _ := cmd.Flags().GetString("confidence")
	typeFilter, _ := cmd.Flags().GetString("type")
	scope, _ := cmd.Flags().GetString("scope")
	project, _ := cmd.Flags().GetString("project")
	tag, _ := cmd.Flags().GetString("tag")
	domain, _ := cmd.Flags().GetString("domain")
	superseded, _ := cmd.Flags().GetBool("superseded")
	limit, _ := cmd.Flags().GetInt("limit")
	pretty, _ := cmd.Flags().GetBool("pretty")

	// Build probe options
	opts := index.ProbeOptions{
		Status:     status,
		Confidence: confidence,
		Type:       typeFilter,
		Scope:      scope,
		Project:    project,
		Tag:        tag,
		Domain:     domain,
		Superseded: superseded,
		Limit:      limit,
	}

	indexer := index.NewSQLiteIndexer(cfg.IndexPath)
	result, err := indexer.Probe(queryStr, opts)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(result)
}

func runContext(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	indexer := index.NewSQLiteIndexer(cfg.IndexPath)
	result, err := indexer.Context()
	if err != nil {
		return fmt.Errorf("context: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(result)
}

func runSQL(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	indexer := index.NewSQLiteIndexer(cfg.IndexPath)
	rows, err := indexer.SQL(args[0])
	if err != nil {
		return fmt.Errorf("sql: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(rows)
}

func runGet(cmd *cobra.Command, args []string) error {
	full, _ := cmd.Flags().GetBool("full")
	metadataOnly, _ := cmd.Flags().GetBool("metadata-only")
	pretty, _ := cmd.Flags().GetBool("pretty")
	format, _ := cmd.Flags().GetString("format")
	titlesOnly, _ := cmd.Flags().GetBool("titles-only")
	section, _ := cmd.Flags().GetString("section")
	sectionSearch, _ := cmd.Flags().GetString("section-search")
	byPaths, _ := cmd.Flags().GetStringSlice("by-path")

	if len(args) == 0 && len(byPaths) == 0 {
		return fmt.Errorf("at least one ID argument or --by-path flag is required")
	}
	if full && metadataOnly {
		return fmt.Errorf("--full and --metadata-only are mutually exclusive")
	}
	if titlesOnly && (full || metadataOnly) {
		return fmt.Errorf("--titles-only cannot be combined with --full or --metadata-only")
	}
	if titlesOnly && (section != "" || sectionSearch != "") {
		return fmt.Errorf("--titles-only cannot be combined with --section or --section-search")
	}
	if metadataOnly && (section != "" || sectionSearch != "") {
		return fmt.Errorf("--metadata-only cannot be combined with --section or --section-search")
	}

	type getResult struct {
		Path              string              `json:"path"`
		Content           string              `json:"content,omitempty"`
		Metadata          *index.Frontmatter  `json:"metadata,omitempty"`
		Error             string              `json:"error,omitempty"`
		Mode              string              `json:"mode,omitempty"`
		Headings          []get.Heading       `json:"headings,omitempty"`
		Sections          []get.SectionResult `json:"sections,omitempty"`
		SelectedSection   string              `json:"selected_section,omitempty"`
		SectionSearchTerm string              `json:"section_search_term,omitempty"`
	}

	results := make([]getResult, 0, len(args)+len(byPaths))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	indexer := index.NewSQLiteIndexer(cfg.IndexPath)

	// Prefer the notes dir stored in the DB during build over the config default.
	// This means `get` works without re-specifying --notes-dir.
	notesDir := indexer.ReadNotesDir()
	if notesDir == "" {
		notesDir = cfg.NotesDir
	}

	type target struct{ resolvedPath, label string }
	var targets []target

	for _, id := range args {
		gr, err := indexer.Get(id)
		if err != nil {
			results = append(results, getResult{Path: id, Error: err.Error()})
			continue
		}
		targets = append(targets, target{
			resolvedPath: filepath.Join(notesDir, gr.Path),
			label:        gr.Path,
		})
	}
	for _, p := range byPaths {
		targets = append(targets, target{resolvedPath: p, label: p})
	}

	for _, tgt := range targets {
		path := tgt.label
		resolvedPath := tgt.resolvedPath

		raw, err := os.ReadFile(resolvedPath)
		if err != nil {
			errMsg := err.Error()
			if os.IsNotExist(err) {
				errMsg = "file not found"
			}
			results = append(results, getResult{Path: path, Error: errMsg})
			continue
		}

		fm, body, err := index.ParseFrontmatter(raw)
		if err != nil {
			results = append(results, getResult{Path: path, Error: fmt.Sprintf("parse error: %v", err)})
			continue
		}

		var r getResult
		r.Path = path
		switch {
		case titlesOnly:
			r.Mode = "titles-only"
			r.Headings = get.ParseHeadings(body)
		case section != "" || sectionSearch != "":
			var secResults []get.SectionResult
			headings := get.ParseHeadings(body)
			lines := strings.Split(body, "\n")

			if section != "" && sectionSearch != "" {
				// union mode
				secA, errA := get.ExtractSection(body, section)
				secB := get.ExtractSectionsContaining(headings, lines, sectionSearch)

				if errA != nil && len(secB) == 0 {
					results = append(results, getResult{Path: path, Error: "section not found and term not found in any section"})
					continue
				}
				var aSlice []get.SectionResult
				if errA == nil {
					aSlice = []get.SectionResult{secA}
				}
				if len(secB) > 0 {
					secResults = get.UnionSections(aSlice, secB, headings)
				} else {
					secResults = aSlice
				}
				r.Mode = "section+section-search"
				r.SelectedSection = section
				r.SectionSearchTerm = sectionSearch
			} else if section != "" {
				sec, err := get.ExtractSection(body, section)
				if err != nil {
					results = append(results, getResult{Path: path, Error: "section not found"})
					continue
				}
				secResults = []get.SectionResult{sec}
				r.Mode = "section"
				r.SelectedSection = section
			} else {
				headings := get.ParseHeadings(body)
				lines := strings.Split(body, "\n")
				secResults = get.ExtractSectionsContaining(headings, lines, sectionSearch)
				if len(secResults) == 0 {
					results = append(results, getResult{Path: path, Error: fmt.Sprintf("term %q not found in document", sectionSearch)})
					continue
				}
				r.Mode = "section-search"
				r.SectionSearchTerm = sectionSearch
			}
			if len(secResults) > 0 {
				r.Sections = secResults
			}
		case metadataOnly:
			r.Mode = "metadata-only"
			r.Metadata = &fm
		case full:
			r.Mode = "full"
			r.Content = body
			r.Metadata = &fm
		default:
			r.Mode = "body"
			r.Content = body
		}
		results = append(results, r)
	}

	if format == "text" {
		for _, r := range results {
			if r.Error != "" {
				fmt.Fprintf(os.Stderr, "error: %s: %s\n", r.Path, r.Error)
				continue
			}
			if metadataOnly {
				enc := json.NewEncoder(os.Stdout)
				fmt.Fprintf(os.Stdout, "%s\t", r.Path)
				_ = enc.Encode(r.Metadata)
			} else {
				fmt.Printf("%s\t%s\n", r.Path, r.Content)
			}
		}
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(results)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
