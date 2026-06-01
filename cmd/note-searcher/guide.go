package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var guideCmd = &cobra.Command{
	Use:   "guide [topic]",
	Short: "Show full usage guide, or a specific topic",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGuide,
}

func runGuide(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Print all topics in sequence with headers
		fmt.Println("note-searcher guide")
		fmt.Println("===================")
		fmt.Println("Run `note-searcher guide <topic>` for a specific topic.")
		fmt.Println()

		topicOrder := []string{"workflow", "probe", "search", "get", "syntax"}
		for _, topic := range topicOrder {
			content, exists := topics[topic]
			if exists {
				fmt.Println(topic)
				fmt.Println(strings.Repeat("-", len(topic)))
				fmt.Print(content)
				fmt.Println()
			}
		}
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

var topics = map[string]string{
	"workflow": `Workflow: probe (wide) -> probe (tighter) xN -> search -> get

Step 1 -- probe wide
  note-searcher probe <broad-query>
  Returns: total_matches + facet counts (tags, types, years, authors).
  Goal: understand the shape of the corpus. No documents returned.

Step 2 -- probe tighter (repeat as needed)
  note-searcher probe <narrower-query>
  Add field filters to reduce total_matches. Repeat until total_matches
  is in a manageable range (typically < 50). Use facet values from the
  previous probe to build the next query.

  When to stop probing:
  - total_matches is small enough to search directly
  - Facets show no useful further refinement
  - You already know the field values you want

  Reading total_matches:
  - Decreases when you add a term: good, the term is narrowing the set.
  - Increases when you add a term: the term is too broad or absent from
    the corpus -- drop it and try a different angle.

Step 3 -- search
  note-searcher search <refined-query> [--limit N] [--score]
  Returns: matching documents with paths, titles, metadata, snippets.
  Use the query you refined through probing.

Step 4 -- get
  note-searcher get <path> [--section <id>] [--section-search <term>]
  Retrieve full content or a specific section of a document.

Notes:
  - All filtering uses Bleve query string syntax (see: guide syntax)
  - Deprecated and superseded notes are hidden by default; use --all to include them
  - Skip probing if you already know what you want

Example end-to-end:
  note-searcher probe "kubernetes rolling update"        # wide: 800 matches
  note-searcher probe "title:rolling update deployment"  # tighter: 45 matches
  note-searcher search "title:rolling update deployment" # get results
  note-searcher get <id>                                 # retrieve full content
`,

	"probe": `probe -- Explore the corpus with facet counts

Usage:
  note-searcher probe [<query>]
  (no query = match all documents)

Output: always JSON. Use --pretty for readable formatting.

What it returns:
  - total_matches: number of documents matching the query
  - Facets: tag counts, type counts, year counts, author counts
  It does NOT return documents. Use search for that.

Flags:
  --pretty    Pretty-print JSON output

Reading total_matches:
  - Decreases when you add a term: good, keep it.
  - Increases when you add a term: that term is too broad or not in the
    corpus -- drop it and try a different angle.
  - Stop when total_matches < 50 or facets show no useful refinement.

Reliable fields:
  title:        reliable -- filter freely
  full-text     reliable -- unqualified terms search content directly

  WARNING: metadata fields are unreliable in many corpora.
  tags:, domain:, author:, status: may be missing or inconsistent.
  Do not use them as filters unless you have confirmed they are populated.
  Stick to title: and full-text terms.

Typical use:
  1. probe with a broad query to see total_matches and dominant facets
  2. Pick a facet value (e.g. a tag) and add it to your next query
  3. Repeat until total_matches is small enough to search

Examples:
  note-searcher probe
  note-searcher probe "kubernetes"
  note-searcher probe "title:kubernetes deployment"
  note-searcher probe "title:kubernetes deployment" --pretty

Query syntax: Bleve query strings (see: guide syntax)
`,

	"search": `search -- Query documents and return results

Usage:
  note-searcher search <query> [flags]

Results are sorted by relevance score (highest first). Use --limit to
cap the number of results returned.

Flags:
  --limit N            Return top N results sorted by relevance (0 = no limit)
  --snippet            Include a text snippet from each result (default: on)
  --snippet-size N     Snippet length in characters; implies --snippet (default 150)
  --sections           Include heading structure in results
  --score              Include relevance score in output
  --all                Include deprecated and superseded documents
  --format             Output format: json or text (default: json)
  --pretty             Pretty-print JSON output

Reliable fields for filtering:
  title:        reliable
  full-text     reliable (unqualified terms)

  WARNING: tags:, domain:, author:, status: are unreliable in many
  corpora and may be missing or inconsistent. Do not filter on them
  unless you have confirmed they are populated. Prefer title: and
  full-text terms.

Default behaviour:
  Output is JSON. Use --format=text for human-readable output.
  Documents with status:deprecated or status:superseded are excluded.
  Use --all to include them.

All filtering uses Bleve query string syntax (see: guide syntax).

Examples:
  note-searcher search "deployment pipeline"
  note-searcher search "title:rolling update deployment" --limit 20
  note-searcher search "kubernetes" --snippet-size 300 --sections
  note-searcher search "deployment" --format=text --pretty
`,

	"get": `get -- Retrieve document content

Usage:
  note-searcher get <path> [<path> ...] [flags]
  note-searcher get --by-path <full-path> [flags]

Modes (flags):
  (no flag)                    Full document content (frontmatter + body)
  --metadata-only              Frontmatter/metadata only, no body
  --titles-only                Heading structure only (H1-H6 with sequential IDs)
  --section <id>               Content of a specific section (e.g. h3)
  --section-search <term>      All sections containing the term (case-insensitive)

Path flags:
  --by-path <path>     Accept a full absolute path (e.g. /home/user/notes/file.md)
                       Useful when piping paths from search results.

Output flags:
  --format             Output format: json or text (default: json)
  --pretty             Pretty-print JSON output

Multiple files:
  Pass multiple paths to retrieve several documents in one call.
  note-searcher get path/a.md path/b.md

Examples:
  note-searcher get notes/deployment.md
  note-searcher get --by-path /home/mads/notes/deployment.md
  note-searcher get notes/deployment.md --section h3
  note-searcher get notes/deployment.md --section-search "rollback"
  note-searcher get notes/a.md notes/b.md --metadata-only
  note-searcher get notes/deployment.md --titles-only
`,

	"syntax": `syntax -- Bleve query string syntax reference

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
  deploy*                 Prefix match (deploy, deployment, deployer...)
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
  - Prefer title: and full-text terms; metadata fields may be unreliable
`,
}
