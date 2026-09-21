#!/bin/bash
# usage: clean.sh out.md file1 file2 ...
out="$1"; shift
: > "$out"
for f in "$@"; do
  echo "" >> "$out"
  echo "#################### $f" >> "$out"
  grep -v '^=\{20,\}$' "$f" | grep -v '^URL: ' | grep -v 'The translations are provided by WeChat Translation' >> "$out"
done
