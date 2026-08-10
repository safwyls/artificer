#!/usr/bin/env bash
# Runs the whole stack locally against a real Dragonwilds server: the
# wkagent sidecar in supervisor mode, and dwcon in front of it.
#
# This is the manual-test path. It is deliberately the same shape as a real
# deployment — the agent owns the game process and dwcon only talks to the
# agent — because Dragonwilds has no admin transport of its own, so anything
# that skips the agent isn't testing the real thing.
#
#   ./scripts/dev-local.sh install   # one-off: SteamCMD the server (~5 GB)
#   ./scripts/dev-local.sh up        # build + start agent and dashboard
#   ./scripts/dev-local.sh start     # ask the agent to start the game
#   ./scripts/dev-local.sh stop      # ask the agent to stop it
#   ./scripts/dev-local.sh status    # health, info, players, metrics
#   ./scripts/dev-local.sh logs      # tail the game log through the agent
#   ./scripts/dev-local.sh down      # stop everything
set -euo pipefail

ROOT="${DWDEV_ROOT:-$HOME/dwtest}"
SERVER_DIR="$ROOT/server"
STEAMCMD_DIR="$ROOT/steamcmd"
RUN_DIR="$ROOT/local"
APP_ID=4019830

# The game refuses to start without an owner. Any non-empty string is
# accepted (verified — see docs/dragonwilds-recon.md), so a placeholder is
# fine until you want to actually join, at which point set DWDEV_OWNER_ID
# to your real Player ID from the game's Settings screen.
OWNER_ID="${DWDEV_OWNER_ID:-test-owner-local}"
AGENT_TOKEN="${DWDEV_AGENT_TOKEN:-local-dev-token-0123456789abcdef}"
ADMIN_PW="${DWDEV_ADMIN_PW:-localadmin123}"
GAME_PORT="${DWDEV_GAME_PORT:-7777}"
AGENT_PORT="${DWDEV_AGENT_PORT:-8811}"
HTTP_PORT="${DWDEV_HTTP_PORT:-8080}"

BASE="http://127.0.0.1:$HTTP_PORT"
AGENT="http://127.0.0.1:$AGENT_PORT"
COOKIES="$RUN_DIR/cookies.txt"
repo_root() { cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd; }

# Short timeouts on purpose: these all talk to loopback, so anything slow
# is something being down, not something being busy. Without them a stopped
# agent takes two minutes to report a connection refusal.
agent_curl() {
  curl -sS --connect-timeout 3 --max-time 120 \
    -H "Authorization: Bearer $AGENT_TOKEN" "$@"
}
api() { curl -sS --connect-timeout 3 --max-time 120 -b "$COOKIES" "$@"; }

# The stack runs as plain background processes, so a WSL restart, a reboot,
# or a `down` leaves nothing behind. Every verb that needs the agent checks
# first and says which thing to run, rather than surfacing a curl error.
require_agent() {
  if ! curl -sS --connect-timeout 2 -o /dev/null "$AGENT/healthz" 2>/dev/null; then
    echo "The wkagent isn't listening on $AGENT." >&2
    echo "It runs as a background process, so a WSL restart or reboot stops it." >&2
    echo "Start the stack with:  $0 up" >&2
    exit 1
  fi
}

login() {
  mkdir -p "$RUN_DIR"
  curl -sS -c "$COOKIES" -X POST "$BASE/api/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" -o /dev/null
}

case "${1:-}" in
install)
  mkdir -p "$STEAMCMD_DIR" "$SERVER_DIR"
  if [ ! -f "$STEAMCMD_DIR/steamcmd.sh" ]; then
    echo "==> fetching steamcmd"
    curl -sSL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz |
      tar zx -C "$STEAMCMD_DIR"
  fi
  # steamcmd is 32-bit and its bundled OpenSSL looks for Debian's CA path.
  # Without both of these it reports "needs to be online" and gives up.
  if [ ! -e /etc/ssl/certs/ca-certificates.crt ]; then
    echo "==> NOTE: steamcmd needs /etc/ssl/certs/ca-certificates.crt."
    echo "    On Fedora:  sudo ln -sf /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem \\"
    echo "                            /etc/ssl/certs/ca-certificates.crt"
    echo "    Also needs 32-bit libs:  sudo dnf install glibc.i686 libstdc++.i686"
  fi
  "$STEAMCMD_DIR/steamcmd.sh" +force_install_dir "$SERVER_DIR" \
    +login anonymous +app_update "$APP_ID" validate +quit
  ;;

up)
  R="$(repo_root)"
  mkdir -p "$RUN_DIR/data"
  echo "==> building"
  (cd "$R" && go build -o "$RUN_DIR/wkagent" ./cmd/wkagent && go build -o "$RUN_DIR/dwcon" ./cmd/dwcon)
  [ -d "$R/web/dist" ] || (cd "$R/web" && npm run build)

  echo "==> starting wkagent (supervisor) on :$AGENT_PORT"
  WKAGENT_MODE=supervisor \
  WKAGENT_TOKEN="$AGENT_TOKEN" \
  WKAGENT_INSTALL_DIR="$SERVER_DIR" \
  WKAGENT_ADDR=":$AGENT_PORT" \
  WKAGENT_GAME_PORT="$GAME_PORT" \
  WKAGENT_STEAMCMD="$STEAMCMD_DIR/steamcmd.sh" \
  WKAGENT_ADMIN_PASSWORD="local-admin-pw" \
  WKAGENT_OWNER_ID="$OWNER_ID" \
  WKAGENT_SERVER_NAME="Grimwood Bastion" \
  WKAGENT_AUTOSTART=false \
    nohup "$RUN_DIR/wkagent" > "$RUN_DIR/agent.log" 2>&1 < /dev/null &

  echo "==> starting dwcon on :$HTTP_PORT"
  JWT_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef" \
  ENCRYPTION_KEY="0123456789abcdef0123456789abcdef" \
  ADMIN_USERNAME=admin ADMIN_PASSWORD="$ADMIN_PW" \
  DATA_DIR="$RUN_DIR/data" HTTP_ADDR=":$HTTP_PORT" \
    nohup "$RUN_DIR/dwcon" > "$RUN_DIR/dwcon.log" 2>&1 < /dev/null &

  sleep 4
  login
  # Register the server once; re-running up is harmless.
  if ! api "$BASE/api/servers" | grep -q '"agentUrl":"'"$AGENT"'"'; then
    echo "==> registering the server row"
    api -X POST "$BASE/api/servers" -H 'Content-Type: application/json' -d "{
      \"name\":\"Grimwood Bastion\",\"host\":\"127.0.0.1\",\"gamePort\":$GAME_PORT,
      \"enabled\":true,\"agentUrl\":\"$AGENT\",\"agentToken\":\"$AGENT_TOKEN\",
      \"savePath\":\"\",\"configPath\":\"\",\"installPath\":\"\",
      \"joinAddress\":\"\",\"containerName\":\"\"}" > /dev/null
  fi
  echo
  echo "    dashboard : $BASE   (admin / $ADMIN_PW)"
  echo "    next      : $0 start"
  ;;

start) require_agent; agent_curl -X POST "$AGENT/v1/power/start" | python3 -m json.tool ;;
stop)  require_agent; agent_curl -X POST "$AGENT/v1/power/stop"  | python3 -m json.tool ;;

status)
  require_agent
  login
  echo "--- agent health ---"; agent_curl "$AGENT/v1/health" | python3 -m json.tool
  echo "--- info ---";    api "$BASE/api/servers/1/info"
  echo; echo "--- players ---"; api "$BASE/api/servers/1/players"
  echo; echo "--- metrics ---"; api "$BASE/api/servers/1/metrics"; echo
  ;;

logs)
  require_agent
  agent_curl "$AGENT/v1/power/logs?tail=${2:-40}" |
    python3 -c 'import json,sys;[print(l) for l in json.load(sys.stdin)["lines"]]'
  ;;

down)
  agent_curl -X POST "$AGENT/v1/power/stop" > /dev/null 2>&1 || true
  # -x matches the process name exactly. -f would also match any shell
  # whose command line mentions these paths, including this script's caller.
  pkill -x wkagent 2>/dev/null || true
  pkill -x dwcon 2>/dev/null || true
  echo "stopped"
  ;;

*)
  sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit 1
  ;;
esac
