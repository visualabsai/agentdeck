#!/usr/bin/env bash
# End-to-end check against a real tmux server: launch an agent, read its status
# out of the pane, message it, kill it. Runs on an isolated tmux socket, so it
# never touches the sessions you are actually working in.
set -euo pipefail

tmp="$(mktemp -d)"
export TMUX_TMPDIR="$tmp"
export AGENTDECK_HOME="$tmp/home"
export PATH="$tmp/bin:$PATH"
unset TMUX || true

cleanup() {
  tmux kill-server 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

fail() { echo "smoke: FAIL: $*" >&2; exit 1; }
ok()   { echo "smoke: ok - $*"; }
# Match against a captured string rather than a pipeline: grep -q closes the
# pipe early, which trips pipefail.
has()  { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

command -v tmux >/dev/null || fail "tmux is not installed"

go build -o "$tmp/agentdeck" .
deck="$tmp/agentdeck"

# A stand-in agent: prints the permission prompt a real CLI prints, then blocks
# on stdin so the session stays alive.
mkdir -p "$tmp/bin" "$tmp/work"
cat > "$tmp/bin/claude" <<'STUB'
#!/bin/sh
echo "starting on: $*"
echo "Do you want to run \`go test ./...\`?"
exec cat
STUB
chmod +x "$tmp/bin/claude"

# agents and help must work with no tmux server running.
out="$("$deck" agents)"
has "$out" "claude" || fail "agents did not list claude"
has "$out" "aider --model ollama_chat/llama3.1" || fail "agents did not list the llama command"
ok "agents"

out="$("$deck" ls)"
[ "$(printf '%s\n' "$out" | wc -l)" -eq 1 ] || fail "expected only a header from ls, got: $out"
ok "empty ls"

name="$("$deck" new claude "$tmp/work" -- "fix the flaky test")"
[ -n "$name" ] || fail "new printed no session name"
tmux has-session -t "=$name" || fail "tmux has no session $name"
ok "launched $name"

sleep 2
out="$("$deck" ls)"
has "$out" "$name" || fail "ls did not list $name"
has "$out" "waiting" || fail "expected status 'waiting', got: $out"
ok "status read from the pane as waiting"

meta="$(cat "$AGENTDECK_HOME/sessions.json")"
has "$meta" '"prompt": "fix the flaky test"' || fail "first prompt was not recorded"
ok "metadata recorded"

"$deck" send "$name" "hello from the smoke test"
sleep 2
pane="$(tmux capture-pane -p -t "$name")"
has "$pane" "hello from the smoke test" || fail "send did not reach the pane"
ok "send"

"$deck" kill "$name"
if tmux has-session -t "=$name" 2>/dev/null; then fail "session survived kill"; fi
meta="$(cat "$AGENTDECK_HOME/sessions.json")"
if has "$meta" "\"$name\""; then fail "metadata survived kill"; fi
ok "kill"

echo "smoke: all checks passed"
