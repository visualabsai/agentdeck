package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/visualabsai/agentdeck/internal/session"
)

func TestShortenCollapsesHomeAndMiddle(t *testing.T) {
	home := "/Users/tester"
	t.Setenv("HOME", home)
	cases := map[string]string{
		home + "/code/crackedjava": "~/code/crackedjava",
		home:                       "~",
		"/var/tmp":                 "/var/tmp",
		"/a/b/c/d/e":               "/…/d/e",
		"~/code/a/b/c":             "~/…/b/c",
	}
	for in, want := range cases {
		if got := shorten(in); got != want {
			t.Errorf("shorten(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Time{}, "-"},
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := ago(c.in); got != c.want {
			t.Errorf("ago(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncateNeverExceedsWidth(t *testing.T) {
	for _, s := range []string{"short", "a much longer session name than fits", "✦ claude-crackedjava", "日本語のセッション名"} {
		for _, w := range []int{0, 1, 4, 10, 80} {
			got := truncate(s, w)
			if lipgloss.Width(got) > w {
				t.Errorf("truncate(%q, %d) = %q (width %d) exceeds %d", s, w, got, lipgloss.Width(got), w)
			}
		}
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate should leave a fitting string alone, got %q", got)
	}
	if got := truncate("anything", 0); got != "" {
		t.Errorf("truncate(_, 0) = %q, want empty", got)
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("ab", 5); lipgloss.Width(got) != 5 {
		t.Errorf("padRight width = %d, want 5", lipgloss.Width(got))
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Errorf("padRight must not truncate, got %q", got)
	}
}

// View must not panic or leak past the terminal for any pane size, including
// the degenerate ones tmux reports while a window is being resized.
func TestViewRendersAtAnySize(t *testing.T) {
	sizes := []struct{ w, h int }{{80, 24}, {200, 60}, {40, 10}, {20, 6}, {1, 1}}
	for _, sz := range sizes {
		m := New()
		m.width, m.height = sz.w, sz.h
		for _, mode := range []mode{modeList, modeNew, modeSend, modeKill} {
			m.mode = mode
			out := m.View()
			for _, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > sz.w {
					t.Errorf("size %dx%d mode %d: line width %d exceeds %d: %q",
						sz.w, sz.h, mode, lipgloss.Width(line), sz.w, line)
				}
			}
		}
	}
}

func TestViewBeforeFirstSizeMessage(t *testing.T) {
	if got := New().View(); got != "loading…" {
		t.Errorf("View() with no size = %q", got)
	}
}

// A deep project path must not wrap the new-session form out of shape.
func TestNewFormFitsWithLongDirectory(t *testing.T) {
	m := New()
	m.width, m.height = 100, 28
	m.mode = modeNew
	m.dirInput.SetValue("/Users/me/work/platform/services/billing-reconciliation/cmd/worker")
	m.promptIn.SetValue(strings.Repeat("make it faster ", 20))

	const w = 61
	for i, line := range strings.Split(m.viewNewForm(w), "\n") {
		if lipgloss.Width(line) > w {
			t.Errorf("form line %d is %d columns, exceeds %d: %q", i, lipgloss.Width(line), w, line)
		}
	}
}

// Pre-filling the directory must park the cursor at the end, or the next
// keystroke lands in the middle of the path.
func TestOpenNewFormPutsCursorAtEndOfDirectory(t *testing.T) {
	long := "/Users/me/code/a-considerably-longer-project-path/service"
	m := New()
	m.width, m.height = 100, 28
	m.sessions = []session.Session{{Name: "s", Dir: long}}
	// Start from a short path so the old cursor lands mid-way through the new
	// one — that is the case bubbles does not fix up for us.
	m.dirInput.SetValue("/tmp")
	m.dirInput.CursorEnd()

	next, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(Model)
	if got.mode != modeNew {
		t.Fatalf("mode = %v, want modeNew", got.mode)
	}
	if got.dirInput.Value() != long {
		t.Fatalf("directory = %q, want %q", got.dirInput.Value(), long)
	}
	if pos := got.dirInput.Position(); pos != len([]rune(long)) {
		t.Errorf("cursor at %d, want %d (end of the path)", pos, len([]rune(long)))
	}
}

func TestQuickLaunchAlsoParksCursorAtEnd(t *testing.T) {
	long := "/Users/me/code/another-long-project-directory-name/api"
	m := New()
	m.width, m.height = 100, 28
	m.sessions = []session.Session{{Name: "s", Dir: long}}
	m.dirInput.SetValue("/tmp")
	m.dirInput.CursorEnd()

	next, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	got := next.(Model)
	if got.agentIdx != 1 {
		t.Errorf("agentIdx = %d, want 1", got.agentIdx)
	}
	if pos := got.dirInput.Position(); pos != len([]rune(long)) {
		t.Errorf("cursor at %d, want %d", pos, len([]rune(long)))
	}
}

// The inputs only scroll horizontally if Update pushes a width into them.
func TestResizeGivesInputsAWidth(t *testing.T) {
	m := New()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := next.(Model)
	if got.dirInput.Width <= 0 || got.promptIn.Width <= 0 || got.sendIn.Width <= 0 {
		t.Fatalf("input widths not set: dir=%d prompt=%d send=%d",
			got.dirInput.Width, got.promptIn.Width, got.sendIn.Width)
	}
	if _, detailW := got.layout(); got.dirInput.Width >= detailW {
		t.Errorf("input width %d should fit inside the detail panel (%d)", got.dirInput.Width, detailW)
	}
}
