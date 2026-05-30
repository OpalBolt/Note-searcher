package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpalBolt/note-searcher/internal/config"
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
	probeCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")

	// get-specific flags
	getCmd.Flags().Bool("full", false, "Return frontmatter + body")
	getCmd.Flags().Bool("metadata-only", false, "Return frontmatter fields only (no body)")
	getCmd.Flags().Bool("pretty", false, "Pretty-print JSON output")
	getCmd.Flags().String("format", "json", "Output format: json or text")

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

	indexer := index.NewBleveIndexer(cfg.IndexPath)
	resp, err := indexer.Search(queryStr, all, limit, snippet, snippetSize)
	if err != nil {
		return fmt.Errorf("search: %w", err)
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
		Path    string         `json:"path"`
		Title   string         `json:"title"`
		Status  string         `json:"status"`
		Domain  []string       `json:"domain"`
		Tags    []string       `json:"tags"`
		Chars   int            `json:"chars"`
		Snippet string         `json:"snippet,omitempty"`
		Matches map[string]int `json:"matches,omitempty"`
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
		out.Results[i] = resultOut{Path: r.Path, Title: r.Title, Status: r.Status, Domain: r.Domain, Tags: r.Tags, Chars: r.Chars, Snippet: r.Snippet, Matches: r.Matches}
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
func runGet(cmd *cobra.Command, args []string) error {
	full, _ := cmd.Flags().GetBool("full")
	metadataOnly, _ := cmd.Flags().GetBool("metadata-only")
	pretty, _ := cmd.Flags().GetBool("pretty")
	format, _ := cmd.Flags().GetString("format")

	if full && metadataOnly {
		return fmt.Errorf("--full and --metadata-only are mutually exclusive")
	}

	type getResult struct {
		Path     string             `json:"path"`
		Content  string             `json:"content,omitempty"`
		Metadata *index.Frontmatter `json:"metadata,omitempty"`
		Error    string             `json:"error,omitempty"`
	}

	results := make([]getResult, 0, len(args))

	for _, path := range args {
		raw, err := os.ReadFile(path)
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
		case metadataOnly:
			r.Metadata = &fm
		case full:
			r.Content = body
			r.Metadata = &fm
		default:
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
