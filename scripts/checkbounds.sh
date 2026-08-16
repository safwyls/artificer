#!/usr/bin/env bash
# Dependency rules for the monorepo, enforced in CI. The full rule set
# lives in docs/unification-plan.md ("Structural rules"); rules that
# only make sense after the Phase 2 restructure are listed at the
# bottom and activate when core/ and games/ exist.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0

# Rule: production code never imports gametest. The test-only game is
# how core proves it is game-agnostic; a production import would make
# the fake load-bearing.
for m in palcon wildskeeper flametender; do
  hits=$(grep -rn --include='*.go' 'internal/game/gametest' "$m/internal" "$m/cmd" 2>/dev/null \
    | grep -v '_test\.go:' || true)
  if [ -n "$hits" ]; then
    echo "BOUNDARY: $m production code imports gametest:"
    echo "$hits"
    fail=1
  fi
done

# Rule: ilmari knows containers, not games or consoles. Neither its
# go.mod nor its source may reference a console module.
hits=$(grep -rn --include='*.go' -E 'safwyls/(palcon|wildskeeper|flametender)' ilmari 2>/dev/null || true)
if [ -n "$hits" ] || grep -qE 'safwyls/(palcon|wildskeeper|flametender)' ilmari/go.mod; then
  echo "BOUNDARY: ilmari references a console module:"
  echo "$hits"
  fail=1
fi

# Phase 2 activations (do not delete; wire up when the restructure lands):
# - core/ never imports games/* or ilmari/
# - game modules under games/ never import each other
# - dockerctl exists only under ilmari/
# - nothing outside tests imports gametest (same rule, new paths)

if [ "$fail" -eq 0 ]; then
  echo "dependency rules: OK"
fi
exit "$fail"
