package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpalBolt/note-searcher/internal/config"
	"github.com/OpalBolt/note-searcher/internal/index"
	"github.com/OpalBolt/note-searcher/internal/search"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "note-searcher",
	Short: "Search and index your notes",
	Long:  "note-searcher is a tool for building an inverted index of your markdown notes and searching them efficiently",
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build the inverted index from the notes directory",
	RunE:  runIndex,
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search notes by full-text and/or metadata filters",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSearch,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)

	// Root persistent flags
	rootCmd.PersistentFlags().String("notes-dir", "./notes", "directory containing markdown notes")
	rootCmd.PersistentFlags().String("index-path", "./notes/.index.json", "path to write the index file")
	rootCmd.PersistentFlags().String("index-type", "json", "index file type (json)")
	rootCmd.PersistentFlags().Int("large-file-threshold", 150, "file size threshold in KB for large file handling")
	indexCmd.Flags().Bool("pretty", false, "write human-readable indented JSON")
	searchCmd.Flags().StringSlice("status", nil, "Filter by status (repeatable)")
	searchCmd.Flags().StringSlice("type", nil, "Filter by type (repeatable)")
	searchCmd.Flags().StringSlice("domain", nil, "Filter by domain (repeatable)")
	searchCmd.Flags().StringSlice("project", nil, "Filter by project (repeatable)")
	searchCmd.Flags().StringSlice("scope", nil, "Filter by scope (repeatable)")
	searchCmd.Flags().StringSlice("confidence", nil, "Filter by confidence (repeatable)")
	searchCmd.Flags().StringSlice("tags", nil, "Filter by tags (repeatable)")
	searchCmd.Flags().Bool("all", false, "Include deprecated and superseded notes")
	searchCmd.Flags().Bool("snippet", false, "Include a content excerpt around the match")
	searchCmd.Flags().String("format", "json", "Output format: json or text")
	searchCmd.Flags().Int("limit", 0, "Maximum number of results (0 = unlimited)")

	// Bind flags to viper
	viper.BindPFlag("notes-dir", rootCmd.PersistentFlags().Lookup("notes-dir"))
	viper.BindPFlag("index-path", rootCmd.PersistentFlags().Lookup("index-path"))
	viper.BindPFlag("index-type", rootCmd.PersistentFlags().Lookup("index-type"))
	viper.BindPFlag("large-file-threshold", rootCmd.PersistentFlags().Lookup("large-file-threshold"))

	// Set up viper to read config file
	setupViper()
}

func setupViper() {
	// Set defaults FIRST
	viper.SetDefault("notes-dir", "./notes")
	viper.SetDefault("index-path", "./notes/.index.json")
	viper.SetDefault("index-type", "json")
	viper.SetDefault("large-file-threshold", 150)

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Get config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fall back to home directory if UserConfigDir fails
		homeDir, err := os.UserHomeDir()
		if err == nil {
			configDir = filepath.Join(homeDir, ".config")
		}
	}

	// Add config path
	if configDir != "" {
		configPath := filepath.Join(configDir, "note-searcher")
		viper.AddConfigPath(configPath)
	}

	// Try to read config file (don't error if it doesn't exist)
	_ = viper.ReadInConfig()
}

func runIndex(cmd *cobra.Command, args []string) error {
	// Load config from viper
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	pretty, _ := cmd.Flags().GetBool("pretty")
	indexer := index.NewJsonIndexer(cfg.IndexPath, pretty)

	// Build index
	stats, err := indexer.Build(cfg.NotesDir)
	if err != nil {
		return fmt.Errorf("build index: %w", err)
	}

	// Print summary
	indexSizeKB := stats.IndexBytes / 1024
	fmt.Printf("Indexed %d files, %d unique terms, index size %d KB\n",
		stats.FileCount, stats.TermCount, indexSizeKB)

	return nil
}

func extractSnippet(path string, terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, term := range terms {
			if strings.Contains(lower, strings.ToLower(term)) {
				start := i - 1
				if start < 0 {
					start = 0
				}
				end := i + 2
				if end > len(lines) {
					end = len(lines)
				}
				parts := make([]string, 0, end-start)
				for _, l := range lines[start:end] {
					l = strings.TrimSpace(l)
					if l != "" {
						parts = append(parts, l)
					}
				}
				return strings.Join(parts, " … ")
			}
		}
	}
	return ""
}

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Build query
	q := index.Query{
		Facets: make(map[string][]string),
	}

	if len(args) > 0 && args[0] != "" {
		q.Terms = index.Tokenize(args[0])
	}

	allFlag, _ := cmd.Flags().GetBool("all")
	q.All = allFlag

	for _, key := range []string{"status", "type", "domain", "project", "scope", "confidence", "tags"} {
		vals, _ := cmd.Flags().GetStringSlice(key)
		if len(vals) > 0 {
			q.Facets[key] = vals
		}
	}

	// Load index from disk
	data, err := os.ReadFile(cfg.IndexPath)
	if err != nil {
		return fmt.Errorf("read index %s: %w", cfg.IndexPath, err)
	}
	var idx index.IndexFile
	if err := index.UnmarshalIndex(data, &idx); err != nil {
		return fmt.Errorf("parse index: %w", err)
	}

	threshold := cfg.LargeFileThreshold
	if threshold == 0 {
		threshold = 150
	}

	engine := search.NewEngine(&idx).WithThreshold(threshold)
	results, err := engine.Search(q)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	format, _ := cmd.Flags().GetString("format")

	if format == "text" {
		for _, r := range results {
			fmt.Printf("%s\t%s\t%s\t%s\n", r.Meta.Path, r.Meta.Title, r.Meta.Status, engine.SizeClass(r.Meta))
		}
		return nil
	}

	// JSON output
	type SearchResult struct {
		Path    string   `json:"path"`
		Title   string   `json:"title"`
		Status  string   `json:"status"`
		Domain  []string `json:"domain"`
		Tags    []string `json:"tags"`
		Size    string   `json:"size"`
		Snippet string   `json:"snippet,omitempty"`
	}

	snippetFlag, _ := cmd.Flags().GetBool("snippet")

	out := make([]SearchResult, 0, len(results))
	for _, r := range results {
		domain := r.Meta.Domain
		if domain == nil {
			domain = []string{}
		}
		tags := r.Meta.Tags
		if tags == nil {
			tags = []string{}
		}
		var snippet string
		if snippetFlag {
			snippet = extractSnippet(r.Meta.Path, q.Terms)
		}
		out = append(out, SearchResult{
			Path:    r.Meta.Path,
			Title:   r.Meta.Title,
			Status:  r.Meta.Status,
			Domain:  domain,
			Tags:    tags,
			Size:    engine.SizeClass(r.Meta),
			Snippet: snippet,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
