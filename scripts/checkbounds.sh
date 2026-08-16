#!/usr/bin/env bash
# Dependency rules for the monorepo, enforced in CI. The full rule set
# lives in docs/unification-plan.md ("Structural rules"). Rules for the
# imported legacy trees stay until each console's port retires its tree.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0

# --- Legacy trees (until their port phases) ---

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

# --- core/ (active since the Phase 2 extraction) ---

# Rule: the same gametest rule, at core's paths.
hits=$(grep -rn --include='*.go' 'core/game/gametest' core 2>/dev/null \
  | grep -v '_test\.go:' | grep -v '^core/game/gametest/' || true)
if [ -n "$hits" ]; then
  echo "BOUNDARY: core production code imports gametest:"
  echo "$hits"
  fail=1
fi

# Rule: core never imports a game module, a console module, or the
# ilmari service — the dependency arrow points at core only.
hits=$(grep -rn --include='*.go' -E '"github.com/safwyls/(sampo/(games|ilmari)|palcon|wildskeeper|flametender|ilmari)' core 2>/dev/null || true)
if [ -n "$hits" ]; then
  echo "BOUNDARY: core imports game/console/ilmari code:"
  echo "$hits"
  fail=1
fi

# Rule: core's docker client is lifecycle-only (the power path for
# adopted containers). Create/remove/pull rights live in ilmari alone —
# a console must never grow the ability to place or unmake containers.
hits=$(grep -rn --include='*.go' -E 'containers/create|images/create|func \(c \*Client\) (ContainerCreate|ContainerRemove|ImagePull)' core/dockerctl 2>/dev/null || true)
if [ -n "$hits" ]; then
  echo "BOUNDARY: core/dockerctl has container create/remove/pull rights (ilmari-only):"
  echo "$hits"
  fail=1
fi

# --- ilmari ---

# Rule: ilmari knows containers, not games or consoles. Neither its
# go.mod nor its source may reference a console module or core.
hits=$(grep -rn --include='*.go' -E 'safwyls/(palcon|wildskeeper|flametender|sampo)' ilmari 2>/dev/null || true)
if [ -n "$hits" ] || grep -qE 'safwyls/(palcon|wildskeeper|flametender|sampo)' ilmari/go.mod; then
  echo "BOUNDARY: ilmari references a console/core module:"
  echo "$hits"
  fail=1
fi

# Rule: game modules never import each other.
for g in games/*/; do
  g=${g%/}
  name=${g#games/}
  hits=$(grep -rn --include='*.go' '"github.com/safwyls/sampo/games/' "$g" 2>/dev/null     | grep -v "games/$name" || true)
  if [ -n "$hits" ]; then
    echo "BOUNDARY: $g imports another game module:"
    echo "$hits"
    fail=1
  fi
done

# Rule: an agent-side game package (games/<g>/<x>agent) never imports its
# console-side game root — the agent binary must not link the game
# registry (see games/enshrouded/esagent's package comment).
for d in games/*/*agent/; do
  [ -d "$d" ] || continue
  parent=$(dirname "${d%/}")
  hits=$(grep -rn --include='*.go' "\"github.com/safwyls/sampo/$parent\"" "$d" 2>/dev/null | grep -v '_test\.go:' || true)
  if [ -n "$hits" ]; then
    echo "BOUNDARY: $d links the console-side game package:"
    echo "$hits"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "dependency rules: OK"
fi
exit "$fail"
