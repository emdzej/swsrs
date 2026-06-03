#!/usr/bin/env bash
#
# End-to-end smoke test:
#   1. build the swsrs binary
#   2. start it with --no-auth
#   3. build the TS workspace (relay client + chat example)
#   4. run `swsrs-chat host` and `swsrs-chat join` against the running relay
#   5. exchange a message in each direction and verify both sides see it
#
# Runs locally and in CI. Requires: go, pnpm, node.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-18099}"
TMP="$(mktemp -d -t swsrs-smoke.XXXXXX)"

cleanup() {
  set +e
  [[ -n "${JOIN_PID:-}" ]] && kill "$JOIN_PID" 2>/dev/null
  [[ -n "${HOST_PID:-}" ]] && kill "$HOST_PID" 2>/dev/null
  [[ -n "${ALICE_KEEP:-}" ]] && kill "$ALICE_KEEP" 2>/dev/null
  [[ -n "${BOB_KEEP:-}" ]] && kill "$BOB_KEEP" 2>/dev/null
  [[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null
  wait 2>/dev/null
  rm -rf "$TMP"
}
trap cleanup EXIT

say() { printf '\033[1;34m[smoke]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[smoke] FAIL:\033[0m %s\n' "$*"; exit 1; }

say "building swsrs binary"
go -C "$ROOT" build -o "$TMP/swsrs" ./cmd/swsrs

say "building TS workspace"
( cd "$ROOT" && pnpm install --frozen-lockfile && pnpm -r run build ) >"$TMP/pnpm.log" 2>&1 \
  || { tail -50 "$TMP/pnpm.log"; fail "pnpm build failed"; }

say "starting relay on :$PORT (--no-auth)"
"$TMP/swsrs" serve --no-auth --addr ":$PORT" >"$TMP/srv.log" 2>&1 &
SRV_PID=$!

# wait for the listening log line
for _ in $(seq 1 30); do
  if grep -q '"msg":"listening"' "$TMP/srv.log" 2>/dev/null; then break; fi
  sleep 0.1
done
grep -q '"msg":"listening"' "$TMP/srv.log" || { cat "$TMP/srv.log"; fail "relay never reported listening"; }

# FIFOs so we can write to each process's stdin without closing it
mkfifo "$TMP/alice.in" "$TMP/bob.in"
# Long-running writers keep the FIFOs' write-end open so readers don't see EOF.
( sleep 60 >"$TMP/alice.in" ) & ALICE_KEEP=$!
( sleep 60 >"$TMP/bob.in" )   & BOB_KEEP=$!

CHAT="$ROOT/examples/chat/dist/index.js"
[[ -f "$CHAT" ]] || fail "chat build artifact missing: $CHAT"

say "starting host (alice)"
node "$CHAT" host \
  --relay "ws://localhost:$PORT" --name alice \
  <"$TMP/alice.in" >"$TMP/alice.out" 2>"$TMP/alice.err" &
HOST_PID=$!

# wait for host to print session details
SESSION=""
TOKEN=""
for _ in $(seq 1 40); do
  if grep -q "responder token:" "$TMP/alice.err" 2>/dev/null; then
    SESSION=$(awk '/session id:/{print $NF}' "$TMP/alice.err")
    TOKEN=$(awk '/responder token:/{print $NF}' "$TMP/alice.err")
    break
  fi
  sleep 0.1
done
[[ -n "$SESSION" && -n "$TOKEN" ]] || { cat "$TMP/alice.err"; fail "host never printed session"; }
say "session=$SESSION"

say "starting join (bob)"
node "$CHAT" join \
  --relay "ws://localhost:$PORT" --session "$SESSION" --token "$TOKEN" --name bob \
  <"$TMP/bob.in" >"$TMP/bob.out" 2>"$TMP/bob.err" &
JOIN_PID=$!

# wait for both sides to log 'connected.'
for _ in $(seq 1 40); do
  if grep -q "connected" "$TMP/alice.err" && grep -q "connected" "$TMP/bob.err"; then break; fi
  sleep 0.1
done

say "exchanging messages"
echo "hi bob, this is alice" > "$TMP/alice.in"
echo "ack from bob"          > "$TMP/bob.in"

# wait for both messages to arrive (up to 5s)
for _ in $(seq 1 50); do
  if grep -q "alice: hi bob" "$TMP/bob.out" && grep -q "bob: ack" "$TMP/alice.out"; then
    say "PASS"
    exit 0
  fi
  sleep 0.1
done

echo "=== alice.out ==="; cat "$TMP/alice.out"
echo "=== bob.out ===";   cat "$TMP/bob.out"
echo "=== alice.err ==="; cat "$TMP/alice.err"
echo "=== bob.err ===";   cat "$TMP/bob.err"
echo "=== srv.log ===";   cat "$TMP/srv.log"
fail "messages never arrived end-to-end"
