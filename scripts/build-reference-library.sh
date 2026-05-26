#!/usr/bin/env bash
# scripts/build-reference-library.sh
#
# Clones kubernetes/website and MicrosoftDocs/azure-docs (shallow + sparse),
# extracts all markdown files into testdata/reference-library/,
# strips existing front-matter, and injects randomised front-matter.
#
# Clones are cached in testdata/tmp/ and reused on subsequent runs.
#
# All file processing is done in a single parallel Python pass — no per-file
# subprocess overhead.
#
# Usage:
#   ./scripts/build-reference-library.sh [--limit N] [--fresh] [--repos REPO1,REPO2]
#
#   --limit N          Only process N markdown files per repo (default: unlimited)
#   --fresh            Delete cached repos and re-clone
#   --repos REPO1,...  Only process specific repos (default: all)
#                      Available: kubernetes-website, azure-docs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="$REPO_ROOT/testdata/reference-library"
CACHE_DIR="$REPO_ROOT/testdata/tmp"
LIMIT=0
FRESH=false
SELECTED_REPOS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --limit) LIMIT="$2"; shift 2 ;;
    --fresh) FRESH=true; shift ;;
    --repos) SELECTED_REPOS="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

declare -A REPOS=(
  ["kubernetes-website"]="https://github.com/kubernetes/website"
  ["azure-docs"]="https://github.com/MicrosoftDocs/azure-docs"
)

# Sparse-checkout paths: only directories that contain real docs markdown
declare -A SPARSE_PATHS=(
  ["kubernetes-website"]="content"
  ["azure-docs"]="articles"
)

mkdir -p "$OUT_DIR" "$CACHE_DIR"

if [[ "$FRESH" == "true" ]]; then
  echo "🗑️  --fresh: Removing cached repos..."
  rm -rf "$CACHE_DIR"/*
fi

echo "📁 Output: $OUT_DIR"
echo "💾 Cache: $CACHE_DIR"
echo "🗑️  Clearing existing markdown files..."
find "$OUT_DIR" -name "*.md" -delete 2>/dev/null || true

CLONE_DIRS=()

# Build list of repos to process
REPOS_TO_PROCESS=()
if [[ -n "$SELECTED_REPOS" ]]; then
  IFS=',' read -ra REPOS_TO_PROCESS <<< "$SELECTED_REPOS"
else
  REPOS_TO_PROCESS=("${!REPOS[@]}")
fi

for name in "${REPOS_TO_PROCESS[@]}"; do
  if [[ -z "${REPOS[$name]:-}" ]]; then
    echo "❌ Unknown repo: $name"
    exit 1
  fi

  url="${REPOS[$name]}"
  sparse="${SPARSE_PATHS[$name]}"
  clone_dir="$CACHE_DIR/$name"

  if [[ -d "$clone_dir/.git" ]]; then
    echo ""
    echo "🔄 Updating $name (cached)..."
    (
      cd "$clone_dir"
      git fetch origin --depth=1
      git reset --hard origin/main
    ) 2>&1 | tail -3
  else
    echo ""
    echo "⬇️  Cloning $url (shallow + sparse: $sparse)..."
    git clone \
      --depth=1 \
      --filter=blob:none \
      --no-checkout \
      "$url" "$clone_dir" 2>&1 | tail -3

    (
      cd "$clone_dir"
      git sparse-checkout init --cone
      git sparse-checkout set $sparse
      git checkout
    ) 2>&1 | tail -2
  fi

  CLONE_DIRS+=("$clone_dir")
  echo "   ✅ Ready: $name"
done

echo ""
echo "⚙️  Processing all markdown files (parallel Python)..."

python3 "$SCRIPT_DIR/inject-frontmatter.py" \
  --out "$OUT_DIR" \
  --limit "$LIMIT" \
  "${CLONE_DIRS[@]}"

FINAL_COUNT=$(find "$OUT_DIR" -name "*.md" | wc -l)
echo ""
echo "🎉 Done! $FINAL_COUNT markdown files in $OUT_DIR"
echo "💡 Tip: Use --fresh to re-download repos, --limit 500 for a quick test, or --repos kubernetes-website for just one"
