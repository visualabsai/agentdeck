package agent

import (
	"regexp"
	"testing"
)

func TestGetKnownAndUnknown(t *testing.T) {
	if a, ok := Get("claude"); !ok || a.Command != "claude" {
		t.Fatalf("Get(claude) = %+v, %v", a, ok)
	}
	if a, ok := Get("CLAUDE"); !ok || a.ID != "claude" {
		t.Fatalf("Get is not case-insensitive: %+v, %v", a, ok)
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("Get(nope) should not resolve")
	}
}

func TestAllIsCompleteAndOrdered(t *testing.T) {
	all := All()
	if len(all) != len(registry) {
		t.Fatalf("All() returned %d agents, registry has %d", len(all), len(registry))
	}
	want := []string{"claude", "codex", "gemini", "opencode", "llama"}
	for i, id := range want {
		if all[i].ID != id {
			t.Errorf("All()[%d].ID = %q, want %q", i, all[i].ID, id)
		}
	}
	for _, a := range all {
		if a.ID == "" || a.Command == "" || a.Icon == "" || a.Color == "" {
			t.Errorf("agent %q has an empty required field: %+v", a.ID, a)
		}
	}
}

func TestByProcessIgnoresAmbiguousRuntimes(t *testing.T) {
	if a, ok := ByProcess("codex"); !ok || a.ID != "codex" {
		t.Errorf("ByProcess(codex) = %+v, %v", a, ok)
	}
	if a, ok := ByProcess("AIDER"); !ok || a.ID != "llama" {
		t.Errorf("ByProcess should be case-insensitive, got %+v, %v", a, ok)
	}
	for _, proc := range []string{"node", "python", "python3", "zsh", ""} {
		if a, ok := ByProcess(proc); ok {
			t.Errorf("ByProcess(%q) should not match, got %q", proc, a.ID)
		}
	}
}

func TestPromptArgs(t *testing.T) {
	claude, _ := Get("claude")
	if got := claude.PromptArgs(""); got != nil {
		t.Errorf("empty prompt should yield no args, got %v", got)
	}
	if got := claude.PromptArgs("fix it"); len(got) != 1 || got[0] != "fix it" {
		t.Errorf("claude.PromptArgs = %v", got)
	}
	gemini, _ := Get("gemini")
	got := gemini.PromptArgs("fix it")
	if len(got) != 2 || got[0] != "-i" || got[1] != "fix it" {
		t.Errorf("gemini.PromptArgs = %v, want [-i, fix it]", got)
	}
}

func TestIDsSorted(t *testing.T) {
	ids := IDs()
	if len(ids) != len(registry) {
		t.Fatalf("IDs() = %v", ids)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("IDs() not sorted: %v", ids)
		}
	}
}

// The status regexes are the only agent-specific part of the tool, so pin the
// lines each CLI actually prints when it blocks on the user.
func TestWaitingPatternsMatchRealPrompts(t *testing.T) {
	cases := []struct{ id, line string }{
		{"claude", "Do you want to run `go test ./...`?"},
		{"claude", "❯ 1. Yes"},
		{"claude", "Allow Bash to run this command?"},
		{"codex", "Allow command?"},
		{"codex", "Approve this edit? [y/n]"},
		{"gemini", "Allow execution of: rm -rf build"},
		{"opencode", "permission required"},
		{"llama", "Add main.go to the chat? [Y/n]"},
	}
	for _, c := range cases {
		a, _ := Get(c.id)
		if !matchAny(a.Waiting, c.line) {
			t.Errorf("%s: no Waiting pattern matched %q", c.id, c.line)
		}
	}
}

func TestIdlePatternsMatchBarePrompts(t *testing.T) {
	cases := []struct{ id, line string }{
		{"claude", "> "},
		{"claude", "? for shortcuts"},
		{"codex", "› "},
		{"gemini", ">"},
		{"opencode", "> "},
	}
	for _, c := range cases {
		a, _ := Get(c.id)
		if !matchAny(a.Idle, c.line) {
			t.Errorf("%s: no Idle pattern matched %q", c.id, c.line)
		}
	}
}

func matchAny(res []*regexp.Regexp, line string) bool {
	for _, re := range res {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
