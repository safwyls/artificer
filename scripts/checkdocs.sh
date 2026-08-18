#!/usr/bin/env bash
# Every doc path a comment points at must exist.
#
# Why this is worth a CI job: the port phases moved code between repos and
# left ~40 comments pointing at documents that never came with it. A
# comment saying "see docs/dragonwilds-recon.md before touching this
# parser" is load-bearing — the recon docs are where the empirically
# verified facts live — and a dangling one is worse than no pointer at
# all, because it reads as though the answer is written down somewhere.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0
# Sources worth checking: our own Go and Markdown, minus vendored trees and
# the embedded per-console doc bundles (those are shipped copies, and their
# internal links resolve inside the bundle rather than in the repo).
mapfile -t files < <(
  find . \( -name '*.go' -o -name '*.md' \) \
    -not -path './node_modules/*' -not -path '*/node_modules/*' \
    -not -path './web/*/dist/*' -not -path './web/*/dist-demo/*' -not -path './cmd/*/docs/*' \
    -not -path './site/*' | sort
)

for f in "${files[@]}"; do
  # Paths written repo-relative, as the comments do: docs/x.md,
  # games/<game>/docs/x.md, anvil/docs/x.md.
  # Lines carrying a <placeholder> describe a path shape rather than a
  # document (docs say games/<game>/docs/recon.md), so they are skipped.
  while read -r ref; do
    [ -z "$ref" ] && continue
    # Repo-relative is how most of these are written; a few (anvil's
    # README) point at a sibling inside their own module.
    [ -f "$ref" ] && continue
    [ -f "$(dirname "$f")/$ref" ] && continue
    echo "$f: points at $ref, which does not exist"
    fail=1
  done < <(grep -v '<[a-z]*>' "$f" |
    grep -oE '\b(docs|[a-z]+/[a-z]+/docs|anvil/docs)/[a-z0-9./-]+\.md' | sort -u)
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "dangling doc references: either bring the document into this repo or fix the pointer"
  exit 1
fi
echo "doc references: OK"
