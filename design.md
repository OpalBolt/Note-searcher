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
- Expected scale: 10,000s of files
- Notes directory and index path configured via `.note-searcher.yaml` (see Configuration)

---

## Query Language

All commands that accept a query (`search`, `probe`) use **Bleve query string syntax**. This is the single, unified way to express queries — no separate structured filter flags.

**Examples:**

```
"error handling"
+golang -deprecated
title:"error handling" +status:verified
domain:kubernetes +tags:ingress
+type:pattern +confidence:high domain:golang
```

**Bleve query string cheatsheet:**

| Syntax | Meaning |
|---|---|
| `term` | Match term anywhere in full-text |
| `+term` | Must include term |
| `-term` | Must exclude term |
| `field:value` | Match specific field |
| `+field:value` | Field must match value |
| `"phrase query"` | Exact phrase match |
| `field:val*` | Wildcard/prefix match |
| `term~` | Fuzzy match |

Metadata fields available for field queries: `title`, `status`, `type`, `scope`, `project`, `domain`, `confidence`, `tags`, `source-agent`, `requires-human-review`.

**Default filters (applied unless explicitly overridden):**
- Excludes `status:deprecated`
- Excludes `status:superseded`
- Excludes notes where `superseded-by` is non-empty

Pass `-status:deprecated` explicitly to override, or use the `--all` flag to disable all default filters.

---

## Commands

### `index`

Build or rebuild the search index from the notes directory.

```
note-searcher index
note-searcher index --notes-dir ./notes --index-path ./.bleve
```

- Runs on demand (scheduled externally, e.g. by an agent or cron)
- No auto-reindex or watch mode in MVP

---

### `search`

Search notes using Bleve query string syntax.

```
note-searcher search "error handling"
note-searcher search "+golang -deprecated"
note-searcher search 'title:"error handling" domain:golang'
note-searcher search "+status:verified domain:kubernetes" --snippet
note-searcher search "+tags:ingress" --all
```

**Default output per result:**
- File path
- `title`, `status`, `domain`, `tags`
- `size`: `small` (≤150 lines) or `large` (>150 lines), threshold configurable

**Flags:**
- `--snippet` — include a content excerpt around the match
- `--all` — include deprecated and superseded notes (hidden by default)
- `--limit` — max results to return
- `--format=text` — plain text output (default: JSON)

---

### `probe`

Probe the search space before committing to a full search. Returns facet counts and all available label values — no result bodies. Always call this first when query scope is unknown.

Accepts the same Bleve query string syntax as `search` — facets are computed over the matching result set, not the whole corpus.

```
note-searcher probe "nginx"
note-searcher probe "+nginx +tags:ingress"
note-searcher probe "+status:verified domain:kubernetes"
```

**Response:**

```json
{
  "query": "+nginx +tags:ingress",
  "total_matches": 70,
  "facets": {
    "status":     { "verified": 31, "inbox": 28, "deprecated": 11 },
    "confidence": { "high": 22, "medium": 35, "low": 13 },
    "domain":     { "kubernetes": 45, "networking": 38, "security": 12 },
    "project":    { "prod-cluster": 29, "staging": 18, "shared": 23 },
    "tags":       { "ingress": 35, "nginx": 28, "tls": 22, "helm": 14, "rbac": 9 }
  }
}
```

- `facets` counts all matches per value for every metadata field — the agent can see exactly what label values exist without guessing
- `total_matches` includes deprecated/superseded notes so the agent understands the full corpus hit — `search` will still hide them unless `--all` is passed

**Intended agent workflow:**
1. `probe "rough query"` → see total hits and facets (~300 tokens)
2. Refine query using facet data
3. `search "refined query" --limit 20` → manageable result set

---

### `get`

Fetch content from one or more notes by file path. Multiple paths can be provided in a single call — all receive the same mode flag.

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
| `--find <term>` | All sections containing the term |
| `--section` + `--find` | Union of both — deduped, ordered by position in file |

**Intended agent workflow:**
1. `search` → get paths + metadata + size for all matches
2. `get file1.md file2.md ... --titles-only` → understand structure in one call
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

Input is flat — the same path may appear multiple times with different modes. The tool merges operations for the same file internally, dedupes sections, and returns one result entry per file ordered by position in the file.

Returns a JSON array with one entry per unique path, in input order. Errors per file are inline (not fatal) so a single bad path doesn't abort the batch.

---

## Output Format

- Default: JSON
- `--format=text`: plain text (no formatting guarantees for agents)
- JSON structure is consistent across all commands

---

## Case Sensitivity

All matching is case-insensitive throughout:

- Full-text search
- `--find <term>` section matching
- `--section <name>` heading matching
- All metadata field values

---

## Configuration

Config file: `~/.config/<projectname>/config.yaml`. All values overridable by flags.

> **Note:** The project name (`note-searcher`) is a placeholder and will change. The config path will update accordingly.

```yaml
notes-dir: ./notes
index-path: ./.bleve
large-file-threshold: 150   # lines
```

---

## Index Backend

Bleve (`github.com/blevesearch/bleve/v2`) is the index backend. The index is stored as a directory (default `.bleve/`).

There is no pluggable backend interface — Bleve is the only implementation.

---

## Must-Have (MVP)

- `index` command (Bleve)
- `search` with Bleve query string syntax + `--snippet` + `--limit` + `--all`
- `probe` with Bleve query string syntax and facets
- `get` with all modes: default, `--full`, `--metadata-only`, `--titles-only`, `--section`, `--find`
- `batch` command
- Default hiding of deprecated/superseded notes
- File size classification in search results
- JSON output + `--format=text`
- `.note-searcher.yaml` config + flag overrides

## Nice-to-Have (Post-MVP)

- Incremental reindex (only changed files)
- Watch mode (auto-reindex on file change)
- `--format=ndjson` streaming output
- Semantic / embedding-based search
- HTTP server / web API mode
- Cross-project search aggregation
