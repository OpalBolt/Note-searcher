#!/usr/bin/env bash
# compare-formats.sh — compare output formats for agent context window cost

BINARY="./bin/note-searcher"
NOTES="./testdata/reference-library"
INDEX="./testdata/bleve"
QUERY="+nginx +tags:ingress"

# ---- helpers ----------------------------------------------------------------

measure() {
  local label="$1" tmpfile
  tmpfile=$(mktemp)
  cat > "$tmpfile"
  local bytes lines tokens
  bytes=$(wc -c < "$tmpfile" | tr -d ' ')
  lines=$(wc -l < "$tmpfile" | tr -d ' ')
  tokens=$(python3 -c "print(int($bytes / 4))")
  printf "  %-30s  %6d bytes  %5d lines  ~%5d tokens\n" "$label" "$bytes" "$lines" "$tokens"
  rm "$tmpfile"
}

section() { echo ""; echo "$1"; echo "------------------------------------------------------"; }

# capture raw JSON once
PROBE_JSON=$(./bin/note-searcher probe   --notes-dir $NOTES --index-path $INDEX "$QUERY" 2>/dev/null)
SEARCH_JSON=$(./bin/note-searcher search --notes-dir $NOTES --index-path $INDEX "$QUERY" 2>/dev/null)

echo "Query: \"$QUERY\""
echo "Results: $(echo "$SEARCH_JSON" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))") search hits"
echo "======================================================"

# ---- PROBE ------------------------------------------------------------------
section "PROBE"
echo "$PROBE_JSON"                                              | measure "pretty json (current)"
echo "$PROBE_JSON" | yq -o=json -I=0 '.'                      | measure "compact json"
echo "$PROBE_JSON" | yq -o=yaml '.'                            | measure "yaml"
echo "$PROBE_JSON" | yq -o=props '.'                           | measure "properties (key=value)"

# ---- SEARCH -----------------------------------------------------------------
section "SEARCH"
echo "$SEARCH_JSON"                                             | measure "pretty json (current)"
echo "$SEARCH_JSON" | yq -o=json -I=0 '.'                     | measure "compact json"
echo "$SEARCH_JSON" | yq -o=yaml '.'                           | measure "yaml"
echo "$SEARCH_JSON" | yq -o=csv '.'  2>/dev/null               | measure "csv"
echo "$SEARCH_JSON" | yq -o=tsv '.'  2>/dev/null               | measure "tsv"
echo "$SEARCH_JSON" | yq -o=props '.'                          | measure "properties (key=value)"
# ndjson: one object per line
echo "$SEARCH_JSON" | python3 -c "
import sys, json
for r in json.load(sys.stdin):
    print(json.dumps(r, separators=(',',':')))
"                                                               | measure "ndjson (1 obj/line)"
# minimal: just the fields an agent actually needs for next step
echo "$SEARCH_JSON" | python3 -c "
import sys, json
for r in json.load(sys.stdin):
    print(json.dumps({'p':r['path'],'t':r['title'],'s':r['status'],'sz':r['size']}, separators=(',',':')))
"                                                               | measure "ndjson minimal (p/t/s/sz)"

# ---- SEARCH --limit 20 -------------------------------------------------------
SEARCH_20=$(./bin/note-searcher search --notes-dir $NOTES --index-path $INDEX "$QUERY" --limit 20 2>/dev/null)
section "SEARCH --limit 20 ($(echo "$SEARCH_20" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))") hits)"
echo "$SEARCH_20"                                               | measure "pretty json (current)"
echo "$SEARCH_20" | yq -o=json -I=0 '.'                       | measure "compact json"
echo "$SEARCH_20" | yq -o=yaml '.'                             | measure "yaml"
echo "$SEARCH_20" | yq -o=csv '.'  2>/dev/null                 | measure "csv"
echo "$SEARCH_20" | yq -o=tsv '.'  2>/dev/null                 | measure "tsv"
echo "$SEARCH_20" | python3 -c "
import sys, json
for r in json.load(sys.stdin):
    print(json.dumps(r, separators=(',',':')))
"                                                               | measure "ndjson (1 obj/line)"
echo "$SEARCH_20" | python3 -c "
import sys, json
for r in json.load(sys.stdin):
    print(json.dumps({'p':r['path'],'t':r['title'],'s':r['status'],'sz':r['size']}, separators=(',',':')))
"                                                               | measure "ndjson minimal (p/t/s/sz)"

# ---- SIDE BY SIDE -----------------------------------------------------------
echo ""
echo "======================================================"
echo "Same first result, every format:"
echo "------------------------------------------------------"
FIRST=$(echo "$SEARCH_JSON" | python3 -c "import sys,json; print(json.dumps([json.load(sys.stdin)[0]]))")

echo "[pretty json]"
echo "$FIRST" | yq -o=json -I=2 '.'
echo "[compact json]"
echo "$FIRST" | yq -o=json -I=0 '.'
echo "[yaml]"
echo "$FIRST" | yq -o=yaml '.' | sed 's/^/  /'
echo "[tsv header+row]"
echo "$FIRST" | yq -o=tsv '.' 2>/dev/null
echo "[ndjson minimal]"
echo "$FIRST" | python3 -c "
import sys, json
r = json.load(sys.stdin)[0]
print(json.dumps({'p':r['path'],'t':r['title'],'s':r['status'],'sz':r['size']}, separators=(',',':')))
"
