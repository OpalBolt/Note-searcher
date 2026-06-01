package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var guideCmd = &cobra.Command{
	Use:   "guide [topic]",
	Short: "Show usage guidance for AI agents and humans",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGuide,
}

func runGuide(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Print(topicIndex)
		return nil
	}

	topic := strings.ToLower(args[0])
	content, exists := topics[topic]
	if !exists {
		validTopics := make([]string, 0, len(topics))
		for k := range topics {
			validTopics = append(validTopics, k)
		}
		sort.Strings(validTopics)
		return fmt.Errorf("unknown topic %q. Valid topics: %s", topic, strings.Join(validTopics, ", "))
	}

	fmt.Print(content)
	return nil
}

var topicIndex = `Available guide topics:

  workflow  Recommended step-by-step usage (start here)
  probe     How to use probe to explore the corpus
  search    Query syntax, flags, and filtering
  get       Retrieving full or partial document content
  syntax    Bleve query string syntax reference

Run: note-searcher guide <topic>
`

var topics = map[string]string{
	"workflow": `Workflow: probe (wide) → probe (tighter) ×N → search → get

Step 1 — probe wide
  note-searcher probe <broad-query>
  Returns: total_matches + facet counts (tags, types, years, authors).
  Goal: understand the shape of the corpus. No documents returned.

Step 2 — probe tighter (repeat as needed)
  note-searcher probe <narrower-query>
  Add field filters to reduce total_matches. Repeat until total_matches
  is in a manageable range (typically < 50). Use facet values from the
  previous probe to build the next query.

  When to stop probing:
  - total_matches is small enough to search directly
  - Facets show no useful further refinement
  - You already know the field values you want

Step 3 — search
  note-searcher search <refined-query> [--limit N] [--snippet] [--score]
  Returns: matching documents with paths, titles, metadata.
  Use the query you refined through probing.

Step 4 — get
  note-searcher get <path> [--section <heading>] [--find <term>]
  Retrieve full content or a specific section of a document.

Notes:
  - All filtering uses Bleve query string syntax (see: guide syntax)
  - Deprecated and superseded notes are hidden by default; use --all to include them
  - Skip probing if you already know what you want
`,

	"probe": `probe — Explore the corpus with facet counts

Usage:
  note-searcher probe <query>

What it returns:
  - total_matches: number of documents matching the query
  - Facets: tag counts, type counts, year counts, author counts
  It does NOT return documents. Use search for that.

Typical use:
  1. probe with a broad query to see total_matches and dominant facets
  2. Pick a facet value (e.g. a tag) and add it to your next query
  3. Repeat until total_matches is small enough to search

Examples:
  note-searcher probe "kubernetes"
  note-searcher probe "kubernetes tags:devops"
  note-searcher probe "kubernetes tags:devops year:2024"

Query syntax: Bleve query strings (see: guide syntax)
`,

	"search": `search — Query documents and return results

Usage:
  note-searcher search <query> [flags]

Flags:
  --limit N       Maximum number of results (default: 10)
  --snippet       Include a text snippet from each result
  --score         Include relevance score in output
  --sort <field>  Sort by field (e.g. date, title)
  --all           Include deprecated and superseded documents
  --format        Output format: json or text (default: text)

Queryable fields:
  title, tags, author, type, year, status, path

Default behaviour:
  Documents with status:deprecated or status:superseded are excluded.
  Use --all to include them.

All filtering uses Bleve query string syntax (see: guide syntax).

Examples:
  note-searcher search "deployment pipeline"
  note-searcher search "tags:devops year:2024" --limit 20 --snippet
  note-searcher search "author:alice" --sort date --score
`,

	"get": `get — Retrieve document content

Usage:
  note-searcher get <path> [<path> ...] [flags]

Modes (flags):
  (no flag)           Full document content
  --metadata-only     Frontmatter/metadata only, no body
  --titles-only       Document title(s) only
  --section <heading> Content under a specific heading
  --find <term>       Sections containing the term

Multiple files:
  Pass multiple paths to retrieve several documents in one call.
  note-searcher get path/a.md path/b.md

Examples:
  note-searcher get notes/deployment.md
  note-searcher get notes/deployment.md --section "Rollback"
  note-searcher get notes/deployment.md --find "canary"
  note-searcher get notes/a.md notes/b.md --metadata-only
`,

	"syntax": `syntax — Bleve query string syntax reference

note-searcher uses Bleve query string syntax for all filtering.
This applies to both probe and search.

Basic terms:
  kubernetes              Match documents containing "kubernetes"
  kubernetes deployment   Match documents containing both terms (AND)

Required / excluded:
  +kubernetes             Term MUST be present
  -deprecated             Term MUST NOT be present

Phrase search:
  "rolling update"        Exact phrase match

Field queries:
  tags:devops             Field equals value
  title:deployment        Match in title field
  author:alice            Match by author
  type:note               Match by document type
  year:2024               Match by year

Wildcard:
  deploy*                 Prefix match (deploy, deployment, deployer…)
  tags:dev*               Prefix match on a field

Fuzzy match:
  kubernets~              Fuzzy match (typo tolerance, default edit distance 1)
  kubernets~2             Fuzzy match with edit distance 2

Combining:
  +tags:devops -status:deprecated "rolling update"
  kubernetes year:2024 author:alice

Notes:
  - Field names are lowercase
  - Wildcards only supported as suffix (prefix wildcards not supported)
  - Phrase search requires double quotes
`,
}
