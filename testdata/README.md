# Reference Library Test Data

Large collection of markdown documents for testing note-searcher performance and accuracy.

## Sources

- **kubernetes/website** — Kubernetes documentation (~2K files)
- **MicrosoftDocs/azure-docs** — Azure documentation (~20K files)

## Generation

All markdown files are:
- Stripped of original YAML front-matter
- Injected with randomized front-matter (title, date, author, status, category, tags)
- Flattened into a single directory with repo-prefixed filenames
- Deterministically generated (same file → same front-matter on re-runs)

## Usage

```bash
# Generate the full library (22K+ files)
./scripts/build-reference-library.sh

# Quick test with limited files
./scripts/build-reference-library.sh --limit 500

# Just Kubernetes docs
./scripts/build-reference-library.sh --repos kubernetes-website

# Fresh clone (re-download repos)
./scripts/build-reference-library.sh --fresh
```

## Performance

- Uses cached clones in `testdata/tmp/` (gitignored)
- Parallel processing with ThreadPoolExecutor (64 workers on 16 CPUs)
- Processes ~22K files in ~2 minutes after initial clone

## Files

- **testdata/reference-library/** — Generated markdown files (gitignored)
- **testdata/tmp/** — Cached git clones (gitignored)
- **scripts/build-reference-library.sh** — Main script
- **scripts/inject-frontmatter.py** — Python worker
