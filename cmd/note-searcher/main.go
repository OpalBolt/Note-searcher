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
	Long:  "note-searcher indexes markdown notes with Bleve and provides fast full-text and metadata search",
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build the Bleve search index from the notes directory",
	RunE:  runIndex,
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search notes using Bleve query string syntax",
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
	Args:  cobra.MinimumNArgs(1),
	RunE:  runGet,
}

func init() {
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(probeCmd)
	rootCmd.AddCommand(getCmd)

	// Persistent flags available to all subcommands
	rootCmd.PersistentFlags().String("notes-dir", "./notes", "directory containing markdown notes")
	rootCmd.PersistentFlags().String("index-path", "./.bleve", "path to the Bleve index directory")

	// search-specific flags
	searchCmd.Flags().Bool("all", false, "Include deprecated and superseded notes")
	searchCmd.Flags().Bool("snippet", false, "Include a content excerpt around the match")
	searchCmd.Flags().Int("snippet-size", 0, "Snippet length in characters; implies --snippet (default 150 when --snippet is used alone)")
	searchCmd.Flags().Bool("score", false, "Include relevance score in output")
	searchCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")
	searchCmd.Flags().String("format", "json", "Output format: json or text")
	searchCmd.Flags().Int("limit", 0, "top N results by relevance score (0 = no limit)")
	searchCmd.Flags().Bool("sections", false, "Include heading structure in search results")
	probeCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")

	// get-specific flags
	getCmd.Flags().Bool("full", false, "Return frontmatter + body")
	getCmd.Flags().Bool("metadata-only", false, "Return frontmatter fields only (no body)")
	getCmd.Flags().Bool("pretty", false, "Pretty-print JSON output")
	getCmd.Flags().String("format", "json", "Output format: json or text")
	getCmd.Flags().Bool("titles-only", false, "Return heading structure only (H1-H6 with sequential IDs)")
	getCmd.Flags().String("section", "", "Return content of section with given ID (e.g. h3)")
	getCmd.Flags().String("section-search", "", "Return all sections containing the given term (case-insensitive)")

	// Bind persistent flags to viper
	_ = viper.BindPFlag("notes-dir", rootCmd.PersistentFlags().Lookup("notes-dir"))
	_ = viper.BindPFlag("index-path", rootCmd.PersistentFlags().Lookup("index-path"))

	setupViper()
}

func setupViper() {
	viper.SetDefault("notes-dir", "./notes")
	viper.SetDefault("index-path", "./.bleve")

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

	indexer := index.NewBleveIndexer(cfg.IndexPath)
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

	all, _ := cmd.Flags().GetBool("all")
	snippet, _ := cmd.Flags().GetBool("snippet")
	snippetSize, _ := cmd.Flags().GetInt("snippet-size")
	if snippetSize > 0 {
		snippet = true
	}
	format, _ := cmd.Flags().GetString("format")
	limit, _ := cmd.Flags().GetInt("limit")
	score, _ := cmd.Flags().GetBool("score")
	sections, _ := cmd.Flags().GetBool("sections")

	indexer := index.NewBleveIndexer(cfg.IndexPath)
	resp, err := indexer.Search(queryStr, all, limit, snippet, snippetSize)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	// Load headings for each result if sections flag is set
	var headingsMap map[string][]get.Heading
	if sections {
		headingsMap = make(map[string][]get.Heading)
		for _, r := range resp.Results {
			filePath := filepath.Join(cfg.NotesDir, filepath.FromSlash(r.Path))
			raw, err := os.ReadFile(filePath)
			if err != nil {
				continue // non-fatal
			}
			_, body, err := index.ParseFrontmatter(raw)
			if err != nil {
				continue // non-fatal
			}
			headingsMap[r.Path] = get.ParseHeadings(body)
		}
	}

	if format == "text" {
		fmt.Printf("Showing %d of %d results\n", resp.Shown, resp.Total)
		for _, r := range resp.Results {
			if score {
				fmt.Printf("%s\t%s\t%s\t%d\t%.4f\n", r.Path, r.Title, r.Status, r.Chars, r.Score)
			} else {
				fmt.Printf("%s\t%s\t%s\t%d\n", r.Path, r.Title, r.Status, r.Chars)
			}
		}
		return nil
	}

	// Strip score — output order already reflects relevance ranking
	type resultOut struct {
		ID       string         `json:"id"`
		Path     string         `json:"path,omitempty"`
		Title    string         `json:"title"`
		Status   string         `json:"status"`
		Domain   []string       `json:"domain"`
		Tags     []string       `json:"tags"`
		Chars    int            `json:"chars"`
		Snippet  string         `json:"snippet,omitempty"`
		Matches  map[string]int `json:"matches,omitempty"`
		Headings []get.Heading  `json:"headings,omitempty"`
	}

	// searchOut wraps search results with metadata about the query.
	// Used when --score=false to exclude scores

	enc := json.NewEncoder(os.Stdout)
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
		enc.SetIndent("", "  ")
	}
	if score {
		// Encode SearchResponse directly to include score
		return enc.Encode(resp)
	}
	// Encode without score - build a wrapper with stripped results
	type searchOut struct {
		Total   int         `json:"total"`
		Shown   int         `json:"shown"`
		Query   string      `json:"query"`
		Results []resultOut `json:"results"`
	}
	out := searchOut{
		Total:   resp.Total,
		Shown:   resp.Shown,
		Query:   resp.Query,
		Results: make([]resultOut, len(resp.Results)),
	}
	for i, r := range resp.Results {
		result := resultOut{ID: r.ID, Title: r.Title, Status: r.Status, Domain: r.Domain, Tags: r.Tags, Chars: r.Chars, Snippet: r.Snippet, Matches: r.Matches}
		if headings, ok := headingsMap[r.Path]; ok {
			result.Headings = headings
		}
		out.Results[i] = result
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

	indexer := index.NewBleveIndexer(cfg.IndexPath)
	result, err := indexer.Probe(queryStr)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(result)
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func runGet(cmd *cobra.Command, args []string) error {
	full, _ := cmd.Flags().GetBool("full")
	metadataOnly, _ := cmd.Flags().GetBool("metadata-only")
	pretty, _ := cmd.Flags().GetBool("pretty")
	format, _ := cmd.Flags().GetString("format")
	titlesOnly, _ := cmd.Flags().GetBool("titles-only")
	section, _ := cmd.Flags().GetString("section")
	sectionSearch, _ := cmd.Flags().GetString("section-search")

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
		Path     string              `json:"path"`
		Content  string              `json:"content,omitempty"`
		Metadata *index.Frontmatter  `json:"metadata,omitempty"`
		Error    string              `json:"error,omitempty"`
		Mode     string              `json:"mode,omitempty"`
		Headings          []get.Heading       `json:"headings,omitempty"`
		Sections          []get.SectionResult `json:"sections,omitempty"`
		SelectedSection   string              `json:"selected_section,omitempty"`
		SectionSearchTerm string              `json:"section_search_term,omitempty"`
	}

	results := make([]getResult, 0, len(args))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	indexer := index.NewBleveIndexer(cfg.IndexPath)
	for _, path := range args {
		resolvedPath := path
		// If path looks like an ID prefix (no path separator, hex chars only, length <= 64)
		if !strings.Contains(path, "/") && !strings.Contains(path, "\\") && isHexString(path) {
			p, err := indexer.ResolveID(path)
			if err != nil {
				return fmt.Errorf("resolve id %q: %w", path, err)
			}
			resolvedPath = filepath.Join(cfg.NotesDir, filepath.FromSlash(p))
		}
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
			if section != "" && sectionSearch != "" {
				// union mode
				secA, errA := get.ExtractSection(body, section)
				secB, errB := get.ExtractSectionsContaining(body, sectionSearch)
				if errA != nil && errB != nil {
					results = append(results, getResult{Path: path, Error: "section not found and term not found in any section"})
					continue
				}
				var aSlice []get.SectionResult
				if errA == nil {
					aSlice = []get.SectionResult{secA}
				}
				if errB == nil {
					headings := get.ParseHeadings(body)
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
				var err error
				secResults, err = get.ExtractSectionsContaining(body, sectionSearch)
				if err != nil {
					results = append(results, getResult{Path: path, Error: "term not found in any section"})
					continue
				}
				r.Mode = "section-search"
				r.SectionSearchTerm = sectionSearch
			}
			r.Sections = secResults
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
