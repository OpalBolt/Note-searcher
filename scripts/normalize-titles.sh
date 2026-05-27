#!/usr/bin/env bash
# Normalize malformed YAML title fields in testdata/reference-library.
#
# Fixes two patterns injected by inject-frontmatter.py:
#   1. HTML anchor tags:  title: "<a name="foo"></a>Real title"
#   2. Inner double-quotes: title: "He said "hello" there"
#
# Strategy: strip HTML tags, then replace remaining inner double-quotes
# with single quotes.

set -euo pipefail

DIR="${1:-testdata/reference-library}"

if [[ ! -d "$DIR" ]]; then
  echo "usage: $0 [notes-dir]" >&2
  exit 1
fi

files=$(grep -rlP '^title: ".*".*"' "$DIR" || true)

if [[ -z "$files" ]]; then
  echo "No files need normalizing."
  exit 0
fi

count=0
while IFS= read -r file; do
  perl -i -pe '
    if (/^title: "(.+)"$/) {
      my $val = $1;
      $val =~ s/<[^>]+>//g;
      $val =~ s/^\s+|\s+$//g;
      $val =~ s/"/'"'"'/g;
      $_ = "title: \"$val\"\n";
    }
  ' "$file"
  ((count++))
done <<< "$files"

echo "Normalized titles in $count file(s)."
