# agentdeck

[![CI](https://github.com/visualabsai/agentdeck/actions/workflows/ci.yml/badge.svg)](https://github.com/visualabsai/agentdeck/actions/workflows/ci.yml)

One dashboard for every CLI coding agent on your machine — Claude Code, Codex CLI,
Gemini CLI, OpenCode, and local models via aider + Ollama.

Each agent runs in its own persistent tmux session. agentdeck lists **every** tmux
session on the PC (agents first), shows live status and a preview of the pane,
and lets you attach, start, message, or kill sessions from one place.

```
 ✦ agentdeck                                        4 sessions · 1 working · 1 waiting  12:04
╭──────────────────────────────────╮╭──────────────────────────────────────────────────────╮
│ ◐ ✦ claude-crackedjava   claude  ││  ✦ Claude Code   claude-crackedjava                  │
│     ~/code/crackedjava · 2m ago  ││ ◐ waiting · ~/code/crackedjava · started 14m ago     │
│ ● ◆ codex-retryd          codex  ││ prompt: fix flaky webhook retry test                 │
│     ~/code/retryd · just now     ││ ──────────────────────────────────────────────────── │
│ ○ ✧ gemini-elec-taxi     gemini  ││ Do you want to run `go test ./...`?                  │
│     ~/code/elec-taxi · 1h ago    ││ ❯ 1. Yes                                             │
│ $   dotfiles                zsh  ││   2. No                                              │
╰──────────────────────────────────╯╰──────────────────────────────────────────────────────╯
 ↑/↓ move · enter attach · n new · 1-5 quick new · s send · x kill · r refresh · q quit
```

Status glyphs: `●` working · `◐` waiting for you · `○` idle at prompt · `$` plain shell.

## Install

agentdeck needs tmux at runtime:

```sh
brew install tmux
```

**Download a binary** (no Go toolchain needed). Pick the build for your Mac —
`arm64` for Apple Silicon, `amd64` for Intel:

```sh
VERSION=0.1.0
ARCH=$([ "$(uname -m)" = "arm64" ] && echo arm64 || echo amd64)
curl -fsSL "https://github.com/visualabsai/agentdeck/releases/download/v${VERSION}/agentdeck_${VERSION}_darwin_${ARCH}.tar.gz" \
  | tar xz agentdeck
sudo mv agentdeck /usr/local/bin/
```

macOS quarantines downloaded binaries, so clear the flag the first time:

```sh
xattr -d com.apple.quarantine /usr/local/bin/agentdeck 2>/dev/null || true
agentdeck version
```

**With Go** (1.22+):

```sh
go install github.com/visualabsai/agentdeck@latest
```

That drops the binary in `$(go env GOPATH)/bin`, which is often not on `PATH`.
Add it once:

```sh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && exec zsh
```

**From source:**

```sh
git clone https://github.com/visualabsai/agentdeck.git
cd agentdeck && go build -o agentdeck . && sudo mv agentdeck /usr/local/bin/
```

Linux works the same way — swap `darwin` for `linux` in the download URL and use
your package manager for tmux.

## Use

```sh
agentdeck                                   # dashboard
agentdeck new claude ~/code/crackedjava -- "fix the flaky test"
agentdeck new codex  ~/code/retryd
agentdeck new llama  .                      # aider on ollama_chat/llama3.1
agentdeck ls
agentdeck attach claude-crackedjava
agentdeck send  claude-crackedjava "yes"
agentdeck kill  claude-crackedjava
```

Inside the dashboard: `↑/↓` or `j/k` move, `enter` attaches (detach with `Ctrl-b d`
to come back), `n` opens the new-session form, `1`–`5` quick-launch an agent in the
selected session's directory, `s` types a message into the selected session without
attaching, `x` kills.

Works from inside tmux too (uses `switch-client` instead of `attach`).

## How status is detected

`internal/agent/agent.go` holds, per agent, the process names tmux reports and
regexes matched against the last lines of the pane. Waiting patterns (permission
prompts, `[y/n]`) win over idle patterns (bare input prompt); otherwise recent pane
activity means working. Adjust the regexes as the CLIs change their UI — they're
the only part that's agent-specific.

## Adding an agent

Add an entry to the registry in `internal/agent/agent.go`: id, command, default
args, a color, and the patterns above. Anything that runs in a terminal works.

## Data

Session metadata (agent, dir, first prompt) lives in `~/.agentdeck/sessions.json`
(override with `AGENTDECK_HOME`). tmux is the source of truth for what's alive.

## Develop

```sh
go test ./...      # unit tests: status detection, tmux parsing, store, layout
./script/smoke.sh  # end-to-end against a real tmux server
go vet ./...
go build -o agentdeck .
```

The packages that talk to tmux are thin on purpose: `tmux.parseList` is tested
against captured `list-sessions` output. Everything below that — launching,
reading a status out of a live pane, sending, killing — is covered by
`script/smoke.sh`, which drives the real binary against a real tmux server on an
isolated socket, so it leaves your own sessions alone. CI runs both.
