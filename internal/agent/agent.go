// Package agent describes the CLI coding agents agentdeck knows how to run.
package agent

import (
	"regexp"
	"sort"
	"strings"
)

// Agent is a CLI coding assistant that can be launched inside a tmux session.
type Agent struct {
	ID      string   // short id used on the command line: claude, codex, ...
	Name    string   // display name
	Command string   // binary to run
	Args    []string // default args
	Color   string   // hex color for the badge in the TUI
	Icon    string   // one-glyph icon

	// Process names tmux reports (#{pane_current_command}) when this agent is
	// running in the foreground. Used to recognise sessions we didn't start.
	ProcessNames []string

	// Patterns matched against the last lines of the pane to detect state.
	// Waiting = agent is blocked on the user (permission prompt, question).
	// Idle    = agent finished and is showing its input prompt.
	Waiting []*regexp.Regexp
	Idle    []*regexp.Regexp
}

// PromptArgs returns the args to pass an initial prompt, if the agent supports it.
func (a Agent) PromptArgs(prompt string) []string {
	if prompt == "" {
		return nil
	}
	switch a.ID {
	case "claude", "codex", "opencode":
		return []string{prompt}
	case "gemini":
		return []string{"-i", prompt}
	default:
		return []string{prompt}
	}
}

var registry = map[string]Agent{
	"claude": {
		ID: "claude", Name: "Claude Code", Command: "claude", Color: "#D97757", Icon: "✦",
		ProcessNames: []string{"claude", "node"},
		Waiting:      rx(`(?i)do you want to`, `(?i)allow .*\?`, `\[y/n\]`, `(?i)press enter to continue`, `❯ 1\. Yes`, `(?i)esc to cancel`),
		Idle:         rx(`^\s*[>❯]\s*$`, `(?i)\? for shortcuts`),
	},
	"codex": {
		ID: "codex", Name: "Codex CLI", Command: "codex", Color: "#10A37F", Icon: "◆",
		ProcessNames: []string{"codex", "node"},
		Waiting:      rx(`(?i)allow command\?`, `(?i)approve`, `\[y/n\]`, `(?i)do you want to`),
		Idle:         rx(`^\s*[>›]\s*$`, `(?i)send a message`),
	},
	"gemini": {
		ID: "gemini", Name: "Gemini CLI", Command: "gemini", Color: "#4285F4", Icon: "✧",
		ProcessNames: []string{"gemini", "node"},
		Waiting:      rx(`(?i)allow execution`, `(?i)\(y/n\)`, `(?i)yes, allow`),
		Idle:         rx(`^\s*>\s*$`, `(?i)type your message`),
	},
	"opencode": {
		ID: "opencode", Name: "OpenCode", Command: "opencode", Color: "#F5A623", Icon: "◈",
		ProcessNames: []string{"opencode"},
		Waiting:      rx(`(?i)permission`, `\[y/n\]`),
		Idle:         rx(`^\s*>\s*$`),
	},
	// Local models via Ollama, driven by aider (OpenAI-compatible endpoint).
	"llama": {
		ID: "llama", Name: "Llama (local)", Command: "aider", Color: "#8E6CEF", Icon: "◉",
		Args:         []string{"--model", "ollama_chat/llama3.1"},
		ProcessNames: []string{"aider", "python", "python3"},
		Waiting:      rx(`\[Y/n\]`, `\(Y\)es/\(N\)o`, `(?i)add .* to the chat\?`),
		Idle:         rx(`^\s*[>]\s*$`, `(?i)^\w*>\s*$`),
	},
}

func rx(pats ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

// Get returns the agent with the given id.
func Get(id string) (Agent, bool) {
	a, ok := registry[strings.ToLower(id)]
	return a, ok
}

// All returns every registered agent in a stable order.
func All() []Agent {
	order := []string{"claude", "codex", "gemini", "opencode", "llama"}
	out := make([]Agent, 0, len(order))
	for _, id := range order {
		out = append(out, registry[id])
	}
	return out
}

// IDs returns registered agent ids, sorted.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ByProcess guesses the agent from a tmux pane's foreground process name.
func ByProcess(proc string) (Agent, bool) {
	proc = strings.ToLower(proc)
	for _, a := range All() {
		for _, p := range a.ProcessNames {
			if p == "node" || p == "python" || p == "python3" {
				continue // too ambiguous on its own
			}
			if proc == p {
				return a, true
			}
		}
	}
	return Agent{}, false
}
