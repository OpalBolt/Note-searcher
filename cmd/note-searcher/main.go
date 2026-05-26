package main

import (
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
	Long:  "note-searcher is a tool for building an inverted index of your markdown notes and searching them efficiently",
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build the inverted index from the notes directory",
	RunE:  runIndex,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(indexCmd)

	// Root persistent flags
	rootCmd.PersistentFlags().String("notes-dir", "./notes", "directory containing markdown notes")
	rootCmd.PersistentFlags().String("index-path", "./notes/.index.json", "path to write the index file")
	rootCmd.PersistentFlags().String("index-type", "json", "index file type (json)")
	rootCmd.PersistentFlags().Int("large-file-threshold", 150, "file size threshold in KB for large file handling")

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

	// Create indexer
	indexer := index.NewJsonIndexer(cfg.IndexPath)

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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
