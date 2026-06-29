#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ] || [ -z "$1" ]; then
  echo "Usage: $0 决策标题" >&2
  exit 1
fi

ADR_DIR="docs/20-architecture/decisions"
mkdir -p "$ADR_DIR"

LAST=$(find "$ADR_DIR" -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]-*.md' \
  | sed -E 's#.*/([0-9]+)-.*#\1#' \
  | sort -n \
  | tail -1 || true)
NEXT=$((10#${LAST:-0} + 1))
NUM=$(printf "%04d" "$NEXT")
SLUG=$(printf '%s' "$1" | sed -E 's/[[:space:]]+/-/g; s#[/\\:*?"<>|]#-#g; s/^-+//; s/-+$//')
if [ -z "$SLUG" ]; then
  SLUG="架构决策"
fi

TARGET="$ADR_DIR/$NUM-$SLUG.md"
if [ -e "$TARGET" ]; then
  echo "ADR already exists: $TARGET" >&2
  exit 1
fi

cp docs/templates/adr.md "$TARGET"
sed -i.bak \
  -e "s/^adr: .*/adr: $NUM/" \
  -e "s/^date: .*/date: $(date +%Y-%m-%d)/" \
  -e "s/^# ADR-NNNN:.*/# ADR-$NUM: $1/" \
  "$TARGET"
rm -f "$TARGET.bak"

echo "Created $TARGET"
