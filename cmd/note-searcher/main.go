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

func init() {
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(probeCmd)

	// Persistent flags available to all subcommands
	rootCmd.PersistentFlags().String("notes-dir", "./notes", "directory containing markdown notes")
	rootCmd.PersistentFlags().String("index-path", "./.bleve", "path to the Bleve index directory")

	// search-specific flags
	searchCmd.Flags().Bool("all", false, "Include deprecated and superseded notes")
	searchCmd.Flags().Bool("snippet", false, "Include a content excerpt around the match")
	searchCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")
	searchCmd.Flags().String("format", "json", "Output format: json or text")
	searchCmd.Flags().Int("limit", 0, "Maximum number of results (0 = unlimited)")
	probeCmd.Flags().Bool("pretty", false, "Pretty-print JSON output (human-readable)")

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
	format, _ := cmd.Flags().GetString("format")
	limit, _ := cmd.Flags().GetInt("limit")

	indexer := index.NewBleveIndexer(cfg.IndexPath)
	results, err := indexer.Search(queryStr, all, limit, snippet)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if format == "text" {
		for _, r := range results {
			fmt.Printf("%s\t%s\t%s\t%d\n", r.Path, r.Title, r.Status, r.Chars)
		}
		return nil
	}

	// Strip score — output order already reflects relevance ranking
	type resultOut struct {
		Path    string   `json:"path"`
		Title   string   `json:"title"`
		Status  string   `json:"status"`
		Domain  []string `json:"domain"`
		Tags    []string `json:"tags"`
		Chars   int      `json:"chars"`
		Snippet string   `json:"snippet,omitempty"`
	}
	out := make([]resultOut, len(results))
	for i, r := range results {
		out[i] = resultOut{r.Path, r.Title, r.Status, r.Domain, r.Tags, r.Chars, r.Snippet}
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
