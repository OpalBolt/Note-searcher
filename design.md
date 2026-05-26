# note-searcher Design

A lightweight Go CLI for indexing and querying markdown note files. Primary consumer is AI agent tool calls — not humans. Output is optimized for minimal context window usage.

## Use Case

AI agents use this tool as a shared memory store — recording past decisions, implementations, patterns, and observations across agent runs. The notes follow the [agent-memory](https://github.com/michaelin/agent-memory) schema.

---

## Note Schema

Frontmatter fields (YAML):

```yaml
title: "Concise factual claim"
created: 2026-05-21
updated: 2026-05-21
status: inbox                  # inbox | verified | deprecated | contested | superseded
confidence: high               # low | medium | high
type: observation              # observation | pattern | constraint | decision | assumption | synthesis
scope: project                 # project | cross-project
project: my-project
domain: [golang, error-handling]
source-agent: my-agent
source-artifact: "pkg/auth/handler.go"
review-by: 2026-08-19
requires-human-review: false
superseded-by: ""
tags: []
```

---

## Corpus

- Single root directory, recursive subdirectory support
- Expected scale: 1,000s of files
- Notes directory and index path configured via `.note-searcher.yaml` (see Configuration)

---

## Commands

### `index`

Build or rebuild the search index from the notes directory.

```
note-searcher index
note-searcher index --notes-dir ./notes --index-path ./notes/.index.json
```

- Runs on demand (scheduled externally, e.g. by an agent or cron)
- No auto-reindex or watch mode in MVP

---

### `search`

Search notes by full-text and/or metadata filters, combinable.

```
note-searcher search "error handling"
note-searcher search "error handling" --type=pattern --domain=golang
note-searcher search --status=verified --project=my-project
note-searcher search "ingress" --snippet
note-searcher search "ingress" --all
```

**Default output per result:**
- File path
- `title`, `status`, `domain`, `tags`
- `size`: `small` (≤150 lines) or `large` (>150 lines), threshold configurable

**Flags:**
- `--snippet` — include a content excerpt around the match
- `--all` — include deprecated and superseded notes (hidden by default)
- `--format=text` — plain text output (default: JSON)
- Metadata filters: `--status`, `--type`, `--domain`, `--project`, `--scope`, `--confidence`, `--tags`; repeatable flags for multi-value e.g. `--tags=ingress --tags=tls`

**Default filters (applied unless `--all`):**
- Hide `status=deprecated`
- Hide `status=superseded`
- Hide notes where `superseded-by` is non-empty

---

### `probe`

Probe the search space before committing to a full search. Returns facet counts, all available label values, and suggested filters — no result bodies. Always call this first when query scope is unknown.

```
note-searcher probe "nginx"
note-searcher probe "nginx" --tags=ingress
note-searcher probe --status=verified --domain=kubernetes
```

**Response:**
```json
{
  "query": { "text": "nginx", "tags": ["ingress"] },
  "total_matches": 70,
  "facets": {
    "status":     { "verified": 31, "inbox": 28, "deprecated": 11 },
    "confidence": { "high": 22, "medium": 35, "low": 13 },
    "domain":     { "kubernetes": 45, "networking": 38, "security": 12 },
    "project":    { "prod-cluster": 29, "staging": 18, "shared": 23 },
    "tags":       { "ingress": 35, "nginx": 28, "tls": 22, "helm": 14, "rbac": 9 }
  },
  "suggested_filters": ["--domain=kubernetes", "--status=verified", "--tags=ingress"]
}
```
- `facets` counts all matches per value for every metadata field, including all `tags` and `domain` values present in the result set — the agent can see exactly what label values exist without guessing
- `suggested_filters` is a starting point based on result distribution (target: reduce to <15 results); the agent should override or extend it based on task context — e.g. replacing `--tags=ingress` with `--tags=ingress --tags=tls` when the task is specifically about TLS termination
- `total_matches` includes deprecated/superseded notes so the agent understands the full corpus hit — `search` will still hide them unless `--all` is passed
- Accepts all the same metadata filter flags as `search` for iterative narrowing

**Intended agent workflow:**
1. `probe` → see total hits, facet distribution, and available label values (~300 tokens)
2. If `total_matches` is high, use `suggested_filters` as a base — adjust using facet labels to match task intent
3. `search` with refined filters + `--limit` → manageable result set

---

### `get`

Fetch content from one or more notes by file path. Multiple paths can be provided in a single call — all receive the same mode flag. Returns a JSON array, one entry per file.

```
note-searcher get ./notes/golang/error-handling.md
note-searcher get ./notes/golang/error-handling.md --full
note-searcher get ./notes/golang/error-handling.md --metadata-only
note-searcher get file1.md file2.md file3.md --titles-only
note-searcher get ./notes/golang/error-handling.md --section "Error Wrapping"
note-searcher get ./notes/golang/error-handling.md --find "ingress"
```

**Modes (mutually exclusive):**

| Flag | Returns |
|---|---|
| _(default)_ | Body only (no frontmatter) |
| `--full` | Body + frontmatter |
| `--metadata-only` | Frontmatter fields only |
| `--titles-only` | Heading structure (H1–H6) only |
| `--section <name>` | Content of a named section (by heading) |
| `--find <term>` | All sections containing the term (by surrounding headings) |
| `--section` + `--find` | Union of both — deduped, ordered by position in file |

**Intended agent workflow:**
1. `search` → get paths + metadata + size for all matches
2. `get file1.md file2.md ... --titles-only` → understand structure of relevant files in one call
3. `batch` with targeted `--section` per file → retrieve only the relevant sections

---

### `batch`

Execute multiple `get` operations in a single call, each with different flags. Accepts a JSON array via stdin or `--input <file>`.

```
note-searcher batch --input operations.json
cat operations.json | note-searcher batch
```

**Input format:**
```json
[
  { "path": "notes/a.md", "mode": "section", "arg": "Ingress" },
  { "path": "notes/b.md", "mode": "section", "arg": "Auth" },
  { "path": "notes/b.md", "mode": "find", "arg": "Ingress" },
  { "path": "notes/c.md", "mode": "titles-only" },
  { "path": "notes/d.md", "mode": "find", "arg": "timeout" }
]
```

Input is flat — the same path may appear multiple times with different modes. The tool merges operations for the same file internally, dedupes sections, and returns one result entry per file ordered by position in the file. The agent does not need to handle merging.

Returns a JSON array with one entry per unique path, in input order. Errors per file are inline (not fatal) so a single bad path doesn't abort the batch.

---

## Output Format

- Default: JSON
- `--format=text`: plain text (cheap to implement, no formatting guarantees for agents)
- JSON structure is consistent across all commands

---

## Case Sensitivity

All matching is case-insensitive throughout the tool:

- `search` full-text matching
- `--find <term>` section matching
- `--section <name>` heading matching
- All metadata filter values (e.g. `--domain=Golang` matches `domain: golang`)

---

## Configuration

Config file: `~/.config/<projectname>/config.yaml`. All values overridable by flags.

> **Note:** The project name (`note-searcher`) is a placeholder and will change. The config path will update accordingly.

```yaml
notes-dir: ./notes
index-path: ./notes/.index.json
large-file-threshold: 150   # lines
```

---

## Index Backend

The index backend is abstracted behind a Go interface, allowing alternative implementations without changing the CLI.

```go
type Indexer interface {
    Build(notesDir string) error
    Search(query Query) ([]Result, error)
    Get(path string) (*Note, error)
}
```

**MVP implementation:** Single JSON file (`.index.json`). Human-readable, no external dependencies, sufficient for 1,000s of files.

**Future backends (nice-to-have):** SQLite, embedded key-value store, etc.

---

## Must-Have (MVP)

- `index` command
- `probe` with facets + `suggested_filters`
- `search` with full-text + metadata filters + `--snippet` + `--limit`
- `get` with all modes: default, `--full`, `--metadata-only`, `--titles-only`, `--section`, `--context`
- Default hiding of deprecated/superseded notes
- File size classification in search results
- JSON output + `--format=text`
- `.note-searcher.yaml` config + flag overrides
- Pluggable indexer interface with JSON implementation

## Nice-to-Have (Post-MVP)

- Incremental reindex (only changed files)
- Watch mode (auto-reindex on file change)
- `--format=ndjson` streaming output
- Semantic / embedding-based search
- HTTP server / web API mode
- Cross-project search aggregation
