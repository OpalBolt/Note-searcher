---
name: note-searcher
description: Query a markdown note corpus using the note-searcher CLI (probe, search, get, batch). Use when searching for notes, retrieving past decisions, querying agent memory, or working with the note-searcher tool or its testdata.
---

# note-searcher

CLI for querying a markdown note corpus. Primary use case: AI agent memory retrieval.

## Testdata flags

```bash
--notes-dir ./testdata/reference-library --index-path ./testdata/bleve
```

---

## Workflow: probe (wide) → probe (tighter) xN → search → get

### Step 1 — probe wide

```bash
note-searcher probe <broad-query>
```

Returns `total_matches` + facet counts (tags, types, years, authors). No documents returned.
Goal: understand the shape of the corpus. Use **1 bare term, 2 at most** — never start with a long multi-term query.

### Step 2 — probe tighter (repeat as needed)

```bash
note-searcher probe <narrower-query>
```

Use `+term` to filter and reduce `total_matches` (bare terms only boost score, they do not narrow results). Repeat until `total_matches` is in a manageable range (typically < 50). Use facet values from the previous probe to build the next query.

**When to stop probing:**
- `total_matches` is small enough to search directly
- Facets show no useful further refinement
- You already know the field values you want

**Reading total_matches:**
- Decreases when you add a `+term`: good, the term is narrowing the set.
- Increases when you add a `+term`: the term is too broad or absent from the corpus — drop it and try a different angle.
- Note: adding a bare term (no `+`) may *increase* `total_matches` — it boosts ranking but does not filter.

### Probe strategy: EXPLORE → DRILL → STOP

1. **EXPLORE** — start with **1 bare term, 2 at most**. Bare terms don't filter, so
   you see the full landscape and discover vocabulary from facet counts.
   Never start with multiple bare terms — the result set will be far too wide.
2. **SWITCH** — once facets reveal a word that exists in the corpus, move to `+` mode immediately.
3. **DRILL** — lock in known terms with `+`, add one at a time, watch `total_matches` fall.
4. **STOP** — when `total_matches` < 50, move to `search`.
5. **EXCLUDE** — use `-term` to cut a dominant but irrelevant domain (e.g. `-azure`)
   when one topic floods the facets and you can't drill past it.

### Step 3 — search

```bash
note-searcher search <refined-query> [--limit N] [--score]
```

Returns matching documents. Each result includes an `id` field — use that `id` with `get` to retrieve full content.

### Step 4 — get

```bash
note-searcher get <id>
```

Pass the `id` from a search result. Retrieves full document content.

**Notes:**
- All filtering uses Bleve query string syntax (see [Query syntax](#query-syntax) below)
- Deprecated and superseded notes are hidden by default; use `--all` to include them
- Skip probing if you already know what you want

### End-to-end example

```bash
note-searcher probe "ACI"                                    # EXPLORE: 1 term, see facets
note-searcher probe "+ACI +azure"                           # SWITCH to + immediately: narrow
note-searcher probe "+ACI +azure +migration"                # DRILL: add one term at a time
note-searcher probe "+ACI +azure +migration +containers"    # DRILL: keep going
note-searcher search "+ACI +azure +migration +containers"   # < 50 matches: search
note-searcher get <id>                                       # retrieve full content by id
```

---

## probe — Explore the corpus with facet counts

**Usage:**
```bash
note-searcher probe [<query>]
# no query = match all documents
```

Output is always JSON.

**What it returns:**
- `total_matches`: number of documents matching the query
- Facets: tag counts, type counts, year counts, author counts
- Does NOT return documents — use `search` for that

**Reading total_matches:**
- Decreases when you add a term: good, keep it.
- Increases when you add a term: that term is too broad or not in the corpus — drop it and try a different angle.
- Stop when `total_matches` < 50 or facets show no useful refinement.

**Reliable fields:**
```
title:        reliable — filter freely
full-text     reliable — unqualified terms search content directly
```

> ⚠️ **WARNING:** metadata fields are unreliable in many corpora.
> `tags:`, `domain:`, `author:`, `status:` may be missing or inconsistent.
> Do not use them as filters unless you have confirmed they are populated.
> Stick to `title:` and full-text terms.

**Typical use:**
1. Probe with a broad query to see `total_matches` and dominant facets
2. Pick a facet value (e.g. a tag) and add it to your next query
3. Repeat until `total_matches` is small enough to search

**Examples:**
```bash
note-searcher probe
note-searcher probe "kubernetes"
note-searcher probe "title:kubernetes deployment"
```

---

## search — Query documents and return results

**Usage:**
```bash
note-searcher search <query> [flags]
```

Results are sorted by relevance score (highest first). Each result includes an `id` — pass it to `get` to retrieve full content.

**Flags:**
```
--limit N            Return top N results sorted by relevance (0 = no limit)
--snippet            Include a text snippet from each result
--snippet-size N     Snippet length in characters; implies --snippet (default 150)
--sections           Include heading structure in results
--score              Include relevance score in output
--all                Include deprecated and superseded documents
```

**Reliable fields for filtering:**
```
title:        reliable
full-text     reliable (unqualified terms)
```

> ⚠️ **WARNING:** `tags:`, `domain:`, `author:`, `status:` are unreliable in many corpora and may be missing or inconsistent. Do not filter on them unless you have confirmed they are populated. Prefer `title:` and full-text terms.

**Default behaviour:**
- Documents with `status:deprecated` or `status:superseded` are excluded by default. Use `--all` to include them.

**Examples:**
```bash
note-searcher search "deployment pipeline"
note-searcher search "title:rolling update deployment" --limit 20
note-searcher search "kubernetes" --snippet-size 300 --sections
```

---

## get — Retrieve document content by ID

**Usage:**
```bash
note-searcher get <id> [<id> ...] [flags]
```

Pass the `id` value from a search result. Multiple IDs can be passed in one call.

**Modes (flags):**
```
(no flag)                    Full document content (frontmatter + body)
--metadata-only              Frontmatter/metadata only, no body
--titles-only                Heading structure only (H1-H6 with sequential IDs)
--section <id>               Content of a specific section (e.g. h3)
--section-search <term>      All sections containing the term (case-insensitive)
```

**Output flags:**
```
--format             Output format: json or text (default: json)
```

**Examples:**
```bash
note-searcher get a3f9c1
note-searcher get a3f9c1 b7e2d4
note-searcher get a3f9c1 --section h3
note-searcher get a3f9c1 --section-search "rollback"
note-searcher get a3f9c1 b7e2d4 --metadata-only
note-searcher get a3f9c1 --titles-only
```

---

## Query syntax

note-searcher uses Bleve query string syntax for all filtering. Applies to both `probe` and `search`.

**Basic terms:**
```
kubernetes              Match documents containing "kubernetes"
kubernetes deployment   Match documents containing both terms (AND)
```

**Required / excluded:**
```
+kubernetes             Term MUST be present (filter — narrows results)
-deprecated             Term MUST NOT be present
```

**Phrase search:**
```
"rolling update"        Exact phrase match
```

**Field queries:**
```
tags:devops             Field equals value
title:deployment        Match in title field
author:alice            Match by author
type:note               Match by document type
year:2024               Match by year
```

**Wildcard:**
```
deploy*                 Prefix match (deploy, deployment, deployer...)
tags:dev*               Prefix match on a field
```

**Fuzzy match:**
```
kubernets~              Fuzzy match (typo tolerance, default edit distance 1)
kubernets~2             Fuzzy match with edit distance 2
```

**Combining:**
```
+tags:devops -status:deprecated "rolling update"
kubernetes year:2024 author:alice
```

**Notes:**
- Field names are lowercase
- Wildcards only supported as suffix (prefix wildcards not supported)
- Phrase search requires double quotes
- Prefer `title:` and full-text terms; metadata fields may be unreliable
