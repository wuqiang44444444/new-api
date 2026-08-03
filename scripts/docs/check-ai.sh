#!/usr/bin/env bash
set -euo pipefail

test -f AGENTS.md
test -f CLAUDE.md
test -f GEMINI.md

grep -q "AGENTS.md" CLAUDE.md
grep -q "AGENTS.md" GEMINI.md
grep -q "docs/README.md" AGENTS.md

echo "ai:check passed"
