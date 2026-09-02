// Package session merges tmux's view of the world with our metadata and
// derives a human-meaningful status for every session on the machine.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/visualabsai/agentdeck/internal/agent"
	"github.com/visualabsai/agentdeck/internal/store"
	"github.com/visualabsai/agentdeck/internal/tmux"
)

// Status of a session's foreground process.
type Status int

const (
	StatusUnknown Status = iota
	StatusWorking        // agent is producing output
	StatusWaiting        // agent is blocked on the user (permission, question)
	StatusIdle           // agent is at its input prompt
	StatusShell          // no agent running, plain shell
)

func (s Status) String() string {
	switch s {
	case StatusWorking:
		return "working"
	case StatusWaiting:
		return "waiting"
	case StatusIdle:
		return "idle"
	case StatusShell:
		return "shell"
	}
	return "unknown"
}

// Session is the merged view.
type Session struct {
	Name     string
	Agent    *agent.Agent // nil for plain shells / unknown processes
	Dir      string
	Process  string
	Status   Status
	Created  time.Time
	Activity time.Time
	Attached bool
	Prompt   string
	Managed  bool // created by agentdeck
	Tail     string
}

// Project is the last path element of Dir.
func (s Session) Project() string {
	return filepath.Base(s.Dir)
}

// Discover lists every tmux session on the machine, classified.
func Discover() ([]Session, error) {
	raw, err := tmux.List()
	if err != nil {
		return nil, err
	}
	metas, _ := store.All()

	out := make([]Session, 0, len(raw))
	for _, r := range raw {
		s := Session{
			Name: r.Name, Dir: r.Path, Process: r.Command,
			Created: r.Created, Activity: r.Activity, Attached: r.Attached,
			Managed: r.AgentDeck,
		}
		if m, ok := metas[r.Name]; ok {
			s.Managed = true
			s.Prompt = m.Prompt
			if a, ok := agent.Get(m.Agent); ok {
				s.Agent = &a
			}
		}
		if s.Agent == nil && r.AgentHint != "" {
			if a, ok := agent.Get(r.AgentHint); ok {
				s.Agent = &a
			}
		}
		if s.Agent == nil {
			if a, ok := agent.ByProcess(r.Command); ok {
				s.Agent = &a
			}
		}
		s.Tail, _ = tmux.Capture(r.Name, 40)
		s.Status = classify(s)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		// agents first, then by most recent activity
		ai, aj := out[i].Agent != nil, out[j].Agent != nil
		if ai != aj {
			return ai
		}
		return out[i].Activity.After(out[j].Activity)
	})
	return out, nil
}

var shells = map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "nu": true}

func classify(s Session) Status {
	if s.Agent == nil {
		if shells[s.Process] {
			return StatusShell
		}
		return StatusUnknown
	}
	// Agent may have exited back to the shell.
	if shells[s.Process] {
		return StatusShell
	}
	lines := lastNonEmpty(s.Tail, 8)
	for _, l := range lines {
		for _, re := range s.Agent.Waiting {
			if re.MatchString(l) {
				return StatusWaiting
			}
		}
	}
	for _, l := range lines {
		for _, re := range s.Agent.Idle {
			if re.MatchString(l) {
				return StatusIdle
			}
		}
	}
	if time.Since(s.Activity) < 4*time.Second {
		return StatusWorking
	}
	return StatusIdle
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func lastNonEmpty(text string, n int) []string {
	all := strings.Split(text, "\n")
	var out []string
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimSpace(ansi.ReplaceAllString(all[i], ""))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Launch starts a new agent session and records it.
func Launch(agentID, dir, name, prompt string) (string, error) {
	a, ok := agent.Get(agentID)
	if !ok {
		return "", fmt.Errorf("unknown agent %q (known: %s)", agentID, strings.Join(agent.IDs(), ", "))
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	abs, err := filepath.Abs(expandHome(dir))
	if err != nil {
		return "", err
	}
	if name == "" {
		name = uniqueName(a.ID, filepath.Base(abs))
	}
	argv := append([]string{a.Command}, a.Args...)
	argv = append(argv, a.PromptArgs(prompt)...)
	if err := tmux.New(name, abs, a.ID, argv); err != nil {
		return "", err
	}
	_ = store.Put(store.Meta{Name: name, Agent: a.ID, Dir: abs, Prompt: prompt, CreatedAt: time.Now()})
	return name, nil
}

// Kill terminates and forgets a session.
func Kill(name string) error {
	if err := tmux.Kill(name); err != nil {
		return err
	}
	return store.Delete(name)
}

func uniqueName(agentID, project string) string {
	project = sanitize(project)
	base := agentID + "-" + project
	name := base
	for i := 2; tmux.Exists(name); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer(".", "-", ":", "-", " ", "-").Replace(s)
	if s == "" {
		return "session"
	}
	return s
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}
