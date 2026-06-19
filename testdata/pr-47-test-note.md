# PR #47 Manual Test Guide

## Setup

```bash
just build
NS=./bin/note-searcher
DB=/tmp/pr47.db
NOTES=./testdata/sample-notes   # or your real notes dir
```

Create a small notes dir if you don't want to use real notes:

```bash
mkdir -p /tmp/pr47-notes
cp testdata/pr-47-test-note.md /tmp/pr47-notes/
NOTES=/tmp/pr47-notes
```

---

## Index

```bash
$NS index --notes-dir $NOTES --index-path $DB
```

Expected: `Indexed N files, index size X KB`

---

## Context

```bash
$NS context --index-path $DB --pretty
```

Expected:
- JSON with `schema`, `samples`, `workflow`, `commands`
- `commands` lists `search` with `--fields`, `--headings`, `--snippet-size`
- No mention of `sql` or file paths in the command reference
- `workflow` says "Never use paths — always use IDs"

---

## Probe

```bash
$NS probe --index-path $DB --pretty
$NS probe "template" --index-path $DB --pretty
$NS probe --status active --index-path $DB --pretty
$NS probe --limit 5 --index-path $DB --pretty
```

Expected: `total_matches` + `facets` (status, type, confidence, project, tags), no body text.

---

## Search — defaults

```bash
$NS search --index-path $DB --pretty
```

Expected:
- Each result has only `id` and `title`
- `id` is short hex (not 64 chars)
- `shown` ≤ 50
- `total` ≥ `shown`

---

## Search — fields

```bash
$NS search --index-path $DB --fields status,tags,confidence --pretty
$NS search --index-path $DB --fields status,type,scope,project,domain --pretty
```

Expected: only requested fields appear alongside `id` and `title`.

---

## Search — headings

```bash
$NS search --index-path $DB --headings --pretty
```

Expected: each result has a `headings` array with `id`, `level`, `text`.

---

## Search — snippets

```bash
# No query → start of file
$NS search --index-path $DB --snippet-size 200 --limit 3 --pretty

# With query → context windows around each hit, joined by …
$NS search "deploy" --snippet-size 150 --index-path $DB --pretty
$NS search "template" --snippet-size 150 --index-path $DB --pretty
```

Expected:
- `snippet` field is non-empty
- Query search: multiple hit windows separated by ` … `
- No-query search: opening lines of the document

---

## Search — total vs shown

```bash
$NS search --limit 3 --index-path $DB | python3 -c "
import sys, json
r = json.load(sys.stdin)
print('total:', r['total'], ' shown:', r['shown'])
assert r['shown'] == 3
assert r['total'] >= r['shown']
print('OK')
"
```

---

## Search — filters

```bash
$NS search --status active --fields status --index-path $DB --pretty
$NS search "deploy" --status active --type note --fields status,type --index-path $DB --pretty

# Superseded excluded by default, included with flag
$NS search --superseded --fields status --index-path $DB --pretty
```

---

## Get — by ID

```bash
# Grab a short ID from search
ID=$($NS search --index-path $DB | python3 -c \
  "import sys,json; print(json.load(sys.stdin)['results'][0]['id'])")
echo "ID: $ID"

$NS get "$ID" --index-path $DB --pretty
$NS get "$ID" --full --index-path $DB --pretty
$NS get "$ID" --metadata-only --index-path $DB --pretty
$NS get "$ID" --titles-only --index-path $DB --pretty
```

Expected:
- `path` is a real relative file path, not the raw ID string
- `content` is non-empty in body/full mode
- `headings` present with `--titles-only`

---

## Get — section navigation

```bash
# Use a heading ID from --titles-only output, e.g. h2
$NS get "$ID" --section h2 --index-path $DB --pretty

# Search within sections
$NS get "$ID" --section-search "deploy" --index-path $DB --pretty

# Document with no headings falls back to whole-body match
```

Expected: sections array with matching content. No "term not found" if the word exists anywhere in the document.

---

## Get — ID behaviour

```bash
# Ambiguous prefix → error
$NS get "a" --index-path $DB

# Non-existent → error
$NS get "0000000" --index-path $DB

# Passing a file path as positional arg → treated as ID, should fail cleanly
$NS get "some/file.md" --index-path $DB
```

---

## SQL

```bash
$NS sql "SELECT count(*) as total FROM docs" --index-path $DB
$NS sql "SELECT path, status FROM docs LIMIT 3" --index-path $DB --pretty

# Empty result → [] not null
$NS sql "SELECT * FROM docs WHERE path = 'nope.md'" --index-path $DB

# Verify _meta was populated
$NS sql "SELECT key, value FROM _meta" --index-path $DB --pretty

# Rejected queries
$NS sql "DELETE FROM docs" --index-path $DB
$NS sql "SELECT 1; DROP TABLE docs" --index-path $DB
```

---

## Guide

```bash
$NS guide syntax
```

Expected: FTS5 syntax reference. No mention of "Bleve".
