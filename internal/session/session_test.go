package session

import (
	"strings"
	"testing"
	"time"

	"github.com/visualabsai/agentdeck/internal/agent"
)

func agentPtr(t *testing.T, id string) *agent.Agent {
	t.Helper()
	a, ok := agent.Get(id)
	if !ok {
		t.Fatalf("unknown agent %q", id)
	}
	return &a
}

func TestClassifyNoAgent(t *testing.T) {
	if got := classify(Session{Process: "zsh"}); got != StatusShell {
		t.Errorf("plain shell = %v, want shell", got)
	}
	if got := classify(Session{Process: "vim"}); got != StatusUnknown {
		t.Errorf("non-shell non-agent = %v, want unknown", got)
	}
}

func TestClassifyAgentExitedToShell(t *testing.T) {
	s := Session{Agent: agentPtr(t, "claude"), Process: "zsh", Tail: "Do you want to continue?"}
	if got := classify(s); got != StatusShell {
		t.Errorf("agent back at the shell = %v, want shell", got)
	}
}

func TestClassifyWaiting(t *testing.T) {
	tail := strings.Join([]string{
		"Running tests…",
		"Do you want to run `go test ./...`?",
		"❯ 1. Yes",
		"  2. No",
	}, "\n")
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: tail, Activity: time.Now()}
	if got := classify(s); got != StatusWaiting {
		t.Errorf("permission prompt = %v, want waiting", got)
	}
}

// A permission prompt is drawn below the input prompt, so both patterns match at
// once; waiting must win or the dashboard hides the session that needs a human.
func TestClassifyWaitingBeatsIdle(t *testing.T) {
	tail := strings.Join([]string{
		"Allow Bash to run this command?",
		"> ",
	}, "\n")
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: tail, Activity: time.Now()}
	if got := classify(s); got != StatusWaiting {
		t.Errorf("= %v, want waiting (idle must not win)", got)
	}
}

func TestClassifyIdleAtPrompt(t *testing.T) {
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: "Done.\n\n> ", Activity: time.Now()}
	if got := classify(s); got != StatusIdle {
		t.Errorf("bare prompt = %v, want idle", got)
	}
}

func TestClassifyWorkingOnRecentActivity(t *testing.T) {
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: "editing main.go", Activity: time.Now()}
	if got := classify(s); got != StatusWorking {
		t.Errorf("recent output with no prompt = %v, want working", got)
	}
}

func TestClassifyStaleOutputIsIdle(t *testing.T) {
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: "editing main.go", Activity: time.Now().Add(-time.Hour)}
	if got := classify(s); got != StatusIdle {
		t.Errorf("stale output = %v, want idle", got)
	}
}

// Panes are full of escape codes; the matcher must see through them.
func TestClassifyStripsANSI(t *testing.T) {
	tail := "\x1b[32m\x1b[1mDo you want to proceed?\x1b[0m"
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: tail, Activity: time.Now()}
	if got := classify(s); got != StatusWaiting {
		t.Errorf("ANSI-wrapped prompt = %v, want waiting", got)
	}
}

// Only the tail of the pane describes the current state; an old prompt further
// up the scrollback must not pin the session to waiting forever.
func TestClassifyOnlyLooksAtTheTail(t *testing.T) {
	lines := []string{"Do you want to run tests?", "yes"}
	for i := 0; i < 20; i++ {
		lines = append(lines, "compiling package", "")
	}
	s := Session{Agent: agentPtr(t, "claude"), Process: "node", Tail: strings.Join(lines, "\n"), Activity: time.Now()}
	if got := classify(s); got != StatusWorking {
		t.Errorf("old prompt in scrollback = %v, want working", got)
	}
}

func TestLastNonEmptySkipsBlanksAndReturnsAtMostN(t *testing.T) {
	got := lastNonEmpty("a\n\nb\n\n\nc\n\n", 2)
	if len(got) != 2 || got[0] != "c" || got[1] != "b" {
		t.Errorf("lastNonEmpty = %q, want [c b]", got)
	}
}

func TestStatusString(t *testing.T) {
	for st, want := range map[Status]string{
		StatusWorking: "working", StatusWaiting: "waiting", StatusIdle: "idle",
		StatusShell: "shell", StatusUnknown: "unknown",
	} {
		if got := st.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", st, got, want)
		}
	}
}

func TestProjectIsLastPathElement(t *testing.T) {
	if got := (Session{Dir: "/Users/me/code/crackedjava"}).Project(); got != "crackedjava" {
		t.Errorf("Project() = %q", got)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"CrackedJava": "crackedjava",
		"my project":  "my-project",
		"a.b:c":       "a-b-c",
		"":            "session",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandHome("~/code"); got != home+"/code" {
		t.Errorf("expandHome(~/code) = %q, want %q", got, home+"/code")
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("expandHome left absolute path alone: %q", got)
	}
}

func TestLaunchRejectsUnknownAgent(t *testing.T) {
	if _, err := Launch("borg", t.TempDir(), "", ""); err == nil {
		t.Fatal("Launch should reject an unknown agent")
	} else if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should list known agents, got %q", err)
	}
}
