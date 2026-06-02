# Search Efficiency — note-searcher vs Naive AI File Search

Corpus: **5,000 markdown files**, 10–500 lines each, avg ~150 lines (~80 tokens/file).  
Full corpus if read: ~400,000 tokens. Context windows top out at 128k–200k. Reading everything is impossible.

---

## Query operator mental model

Before running probes, understand how operators affect result counts:

| Operator | Meaning | Effect on `total_matches` |
|----------|---------|--------------------------|
| `+term`  | Filter — document **must** contain this word | Decreases (narrows) |
| `term`   | Boost — nice to have, affects ranking only | May **increase** or stay flat |
| `-term`  | Exclude — document must **not** contain this word | Decreases (narrows) |

**Key consequence:** `dns +kubernetes` returns every kubernetes doc — `dns` only boosts score, it does not filter. Use `+dns +kubernetes` to require both terms.

## 5-step probe strategy

```
1. EXPLORE  — bare terms to discover vocabulary from facet counts
              (doesn't filter, shows full landscape)
2. SWITCH   — once facets confirm a word exists in the corpus, move to + mode
3. DRILL    — add +terms one at a time, watch total_matches fall
4. STOP     — when total_matches < 50, move to search
5. EXCLUDE  — use -term to cut a dominant irrelevant domain (e.g. -azure)
              when one topic floods facets and you can't drill past it
```

**Canonical example:**
```bash
note-searcher probe "dns"                              # EXPLORE — see facets
note-searcher probe "+dns +kubernetes"                 # DRILL — 293 matches
note-searcher probe "+dns +kubernetes +troubleshoot"   # DRILL — 30 matches → ready
note-searcher search "+dns +kubernetes +troubleshoot"  # search
```

## How AI tools normally search large corpora

Most AI coding tools (Cursor, Copilot, etc.) approach large file sets in one of two ways:

### Option A — Embedding / semantic search
Pre-embed all files, retrieve top-N by vector similarity. Works well for code. Struggles with sparse or domain-specific notes where exact metadata (status, confidence, project) matters as much as content similarity. Also requires an embedding pipeline and model — external dependency.

### Option B — Grep + cat (tool-call loop)
The agent is given bash access and does something like:
```bash
grep -rl "kubernetes" ./notes/    # finds 800 files
cat notes/k8s/ingress-setup.md    # 400 lines
cat notes/k8s/helm-patterns.md    # 300 lines
# ... agent gives up or hits context limit
```

No awareness of staleness (deprecated/superseded notes read anyway), no partial reads, no size awareness. Each `cat` of a 300-line file costs ~240 tokens. After 10 files the agent has burned ~2,400 tokens on raw I/O and hasn't done any reasoning yet.

---

## Example 1 — Wide search: `kubernetes`

### Naive grep + cat

```bash
grep -rl "kubernetes" ./notes/
# returns ~800 files
```

The agent has 800 paths and no good move. It starts reading:

| Action | Tokens |
|---|---|
| File list (800 paths) | ~3,200 |
| Read 10 files at avg 200 lines each | ~16,000 |
| Stale/deprecated notes mixed in | wasted |
| **Total before any reasoning** | **~19,000+** |

Result: agent hits context pressure, starts dropping files, misses things, or asks the user to narrow down — which defeats the purpose of autonomous search.

---

### With note-searcher

**Step 1 — Probe**
```bash
note-searcher probe "kubernetes"
```
```json
{
  "total_matches": 800,
  "facets": {
    "status":     { "verified": 310, "inbox": 290, "deprecated": 200 },
    "confidence": { "high": 280, "medium": 340, "low": 180 },
    "domain":     { "networking": 310, "storage": 180, "security": 140, "ci-cd": 170 },
    "project":    { "prod-cluster": 420, "staging": 230, "shared": 150 },
    "tags":       { "ingress": 180, "nginx": 140, "tls": 95, "helm": 80, "rbac": 60 }
  },
  "suggested_filters": ["--domain=networking", "--status=verified", "--tags=ingress"]
}
```
**~300 tokens.** Agent sees 800 is too broad. `suggested_filters` recommends `--tags=ingress` but the agent can see from the facets that `tls` has 95 hits — if the task is about TLS termination it would use `--tags=ingress --tags=tls` instead, or swap to just `--tags=tls`.

---

**Step 2 — Refined search**
```bash
note-searcher search "kubernetes" --domain=networking --status=verified --confidence=high --limit=10
```
```json
[
  { "path": "notes/k8s/ingress-nginx-setup.md",       "title": "nginx ingress requires SSL annotation",            "status": "verified", "domain": ["networking", "kubernetes"], "tags": ["ingress", "nginx"], "size": "small" },
  { "path": "notes/k8s/ingress-class-conflicts.md",   "title": "Multiple ingress classes silently drop routes",    "status": "verified", "domain": ["networking", "kubernetes"], "tags": ["ingress", "debugging"], "size": "large" },
  { "path": "notes/k8s/network-policy-defaults.md",   "title": "Default network policy blocks all egress",        "status": "verified", "domain": ["networking", "security"],   "tags": ["network-policy"], "size": "small" },
  { "path": "notes/k8s/service-mesh-tradeoffs.md",    "title": "Istio vs Linkerd latency tradeoffs",              "status": "verified", "domain": ["networking"],               "tags": ["istio", "linkerd"], "size": "large" },
  { "path": "notes/k8s/dns-resolution-lag.md",        "title": "CoreDNS cache lag causes stale service lookups",  "status": "verified", "domain": ["networking", "debugging"],  "tags": ["coredns", "dns"], "size": "small" }
  // ...5 more
]
```
**~1,000 tokens.** 3 small files safe to read whole. 2 large files need scoping.

---

**Step 3 — Titles-only on large files**
```bash
note-searcher get notes/k8s/ingress-class-conflicts.md notes/k8s/service-mesh-tradeoffs.md --titles-only
```
```json
[
  { "path": "notes/k8s/ingress-class-conflicts.md", "titles": ["# Ingress Class Conflicts", "## Root Cause", "## Symptoms", "## Fix", "## Related Issues"] },
  { "path": "notes/k8s/service-mesh-tradeoffs.md",  "titles": ["# Service Mesh Tradeoffs", "## Istio", "## Linkerd", "## Recommendation", "## Benchmarks"] }
]
```
**~150 tokens.**

---

**Step 4 — Targeted batch fetch**
```bash
note-searcher batch
[
  { "path": "notes/k8s/ingress-class-conflicts.md", "mode": "section", "arg": "Fix" },
  { "path": "notes/k8s/service-mesh-tradeoffs.md",  "mode": "section", "arg": "Recommendation" },
  { "path": "notes/k8s/ingress-nginx-setup.md",     "mode": "default" },
  { "path": "notes/k8s/network-policy-defaults.md", "mode": "default" },
  { "path": "notes/k8s/dns-resolution-lag.md",      "mode": "default" }
]
```
**~1,200 tokens** for all five files (3 full small files + 2 targeted sections).

---

### Wide search comparison

| | Naive grep + cat | note-searcher |
|---|---|---|
| Tokens spent | ~19,000+ | **~2,650** |
| Stale notes read | ✅ yes | ❌ filtered |
| Files missed due to context pressure | likely | none |
| Agent reasoning quality | degraded | full context available |

---

## Example 2 — Specific search: `pi` (the coding harness)

A narrow query in a 5,000-file corpus. Maybe 8 relevant files exist.

### Naive grep + cat

```bash
grep -rl "pi" ./notes/
# returns thousands — "pi" appears in everything
grep -rl "\bpi\b" ./notes/
# still noisy — matches "pi" in unrelated contexts
```

The agent now has to read files to figure out which ones are actually about the `pi` coding harness vs. math constants, Raspberry Pi, or passing mentions. It burns tokens disambiguating, not learning.

---

### With note-searcher

**Step 1 — Probe**
```bash
note-searcher probe "pi" --tags=coding-harness
```
```json
{
  "total_matches": 8,
  "facets": {
    "status":     { "verified": 5, "inbox": 3 },
    "confidence": { "high": 5, "medium": 3 },
    "domain":     { "tooling": 6, "workflow": 2 },
    "project":    { "shared": 8 },
    "tags":       { "coding-harness": 8, "extensions": 4, "skills": 3, "sdk": 2, "themes": 2 }
  },
  "suggested_filters": []
}
```
**~200 tokens.** 8 results, no filtering needed — agent goes straight to search.

---

**Step 2 — Search**
```bash
note-searcher search "pi" --tags=coding-harness
```
```json
[
  { "path": "notes/tooling/pi-overview.md",          "title": "pi is a terminal-native AI coding harness",              "status": "verified", "size": "large" },
  { "path": "notes/tooling/pi-skills.md",            "title": "pi skills let agents load specialised instructions",     "status": "verified", "size": "small" },
  { "path": "notes/tooling/pi-extensions.md",        "title": "pi extensions expose custom tools to the agent",        "status": "verified", "size": "small" },
  { "path": "notes/tooling/pi-agent-workflow.md",    "title": "Recommended agent loop pattern for pi",                 "status": "inbox",   "size": "large" },
  { "path": "notes/tooling/pi-context-limits.md",    "title": "pi sessions have no persistent memory across runs",     "status": "verified", "size": "small" },
  { "path": "notes/tooling/pi-themes.md",            "title": "pi themes are cosmetic only, no agent behaviour change","status": "verified", "size": "small" },
  { "path": "notes/tooling/pi-prompt-templates.md",  "title": "Prompt templates reduce repetitive system prompt setup","status": "inbox",   "size": "small" },
  { "path": "notes/tooling/pi-sdk.md",               "title": "pi SDK allows programmatic session control",            "status": "verified", "size": "large" }
]
```
**~800 tokens.** All 8 results, 5 small files readable whole, 3 large need scoping.

---

**Step 3 — Titles-only on large files**
```bash
note-searcher get notes/tooling/pi-overview.md notes/tooling/pi-agent-workflow.md notes/tooling/pi-sdk.md --titles-only
```
**~180 tokens.**

---

**Step 4 — Targeted batch**

Agent picks sections it actually needs, ignores `pi-themes.md` as irrelevant to its task.

**~900 tokens** for all relevant content.

---

### Specific search comparison

| | Naive grep + cat | note-searcher |
|---|---|---|
| Tokens spent | ~3,000–5,000 (disambiguation noise) | **~2,080** |
| False positives (wrong "pi") | many | none — tag filtered |
| Agent had to disambiguate | ✅ yes | ❌ no |
| All 8 relevant files found | maybe | ✅ yes |

---

## Overall comparison

| Scenario | Naive | note-searcher | Saving |
|---|---|---|---|
| Wide: `kubernetes` (800 hits) | ~19,000 tokens | ~2,650 tokens | **~86%** |
| Specific: `pi` (8 hits, noisy term) | ~3,000–5,000 tokens | ~2,080 tokens | **~50–60%** |

The saving is smallest on narrow, clean queries — and largest exactly where you need it most: broad searches across a large corpus where naive approaches collapse.

At 5,000 files, the difference between a working agent and a context-blown one is almost entirely about not reading files you don't need.
