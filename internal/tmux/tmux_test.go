package tmux

import (
	"strings"
	"testing"
	"time"
)

func row(fields ...string) string { return strings.Join(fields, sep) }

func TestParseList(t *testing.T) {
	out := strings.Join([]string{
		row("claude-api", "/Users/me/code/api", "node", "4242", "1700000000", "1700000600", "1700000900", "0", "1", "1", "claude"),
		row("dotfiles", "/Users/me/dotfiles", "zsh", "77", "1700000000", "1700000001", "1700000001", "1", "3", "", ""),
		"",
	}, "\n")

	got := parseList(out)
	if len(got) != 2 {
		t.Fatalf("parsed %d sessions, want 2", len(got))
	}

	a := got[0]
	if a.Name != "claude-api" || a.Path != "/Users/me/code/api" || a.Command != "node" {
		t.Errorf("session fields = %+v", a)
	}
	if a.PID != 4242 || a.Windows != 1 {
		t.Errorf("PID/Windows = %d/%d", a.PID, a.Windows)
	}
	if !a.Created.Equal(time.Unix(1700000000, 0)) || !a.Activity.Equal(time.Unix(1700000900, 0)) {
		t.Errorf("timestamps = %v / %v", a.Created, a.Activity)
	}
	if a.Attached {
		t.Error("session_attached=0 should not be attached")
	}
	if !a.AgentDeck || a.AgentHint != "claude" {
		t.Errorf("agentdeck marker not read: %+v", a)
	}

	b := got[1]
	if !b.Attached {
		t.Error("session_attached=1 should be attached")
	}
	if b.AgentDeck || b.AgentHint != "" {
		t.Errorf("unmarked session should have no agent hint: %+v", b)
	}
}

// A session name or path may contain almost anything; only the separator
// between fields is ours, so extra fields must not shift the parse.
func TestParseListToleratesOddNames(t *testing.T) {
	got := parseList(row("my session (2)", "/tmp/a b", "zsh", "1", "1", "1", "1", "0", "1", "", ""))
	if len(got) != 1 || got[0].Name != "my session (2)" || got[0].Path != "/tmp/a b" {
		t.Fatalf("parsed = %+v", got)
	}
}

func TestParseListSkipsShortAndEmptyLines(t *testing.T) {
	out := strings.Join([]string{
		"",
		row("truncated", "/tmp", "zsh"),
		row("good", "/tmp", "zsh", "1", "1", "1", "1", "0", "1", "", ""),
	}, "\n")
	got := parseList(out)
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("parsed = %+v, want only the complete row", got)
	}
}

// session_activity is frozen at creation time for the detached sessions
// agentdeck runs, so a session that is producing output must take its activity
// from window_activity or it can never be reported as working.
func TestParseListPrefersWindowActivity(t *testing.T) {
	got := parseList(row("busy", "/tmp", "node", "1", "100", "100", "900", "0", "1", "1", "codex"))
	if len(got) != 1 {
		t.Fatalf("parsed %d sessions", len(got))
	}
	if !got[0].Activity.Equal(time.Unix(900, 0)) {
		t.Errorf("Activity = %v, want the window_activity value %v", got[0].Activity, time.Unix(900, 0))
	}
}

// The reverse also holds: an attached, quiet session has the later
// session_activity, and that must win.
func TestParseListPrefersSessionActivityWhenLater(t *testing.T) {
	got := parseList(row("quiet", "/tmp", "zsh", "1", "100", "900", "100", "1", "1", "", ""))
	if !got[0].Activity.Equal(time.Unix(900, 0)) {
		t.Errorf("Activity = %v, want %v", got[0].Activity, time.Unix(900, 0))
	}
}

func TestParseListEmptyOutput(t *testing.T) {
	if got := parseList(""); len(got) != 0 {
		t.Fatalf("parseList(\"\") = %+v", got)
	}
}

func TestAttachCmdUsesSwitchClientInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if got := AttachCmd("s").Args; got[1] != "attach-session" {
		t.Errorf("outside tmux: %v", got)
	}
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	if got := AttachCmd("s").Args; got[1] != "switch-client" {
		t.Errorf("inside tmux: %v", got)
	}
}
