// Package tmux is a thin wrapper over the tmux CLI.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Session is a live tmux session as reported by tmux itself.
type Session struct {
	Name      string
	Path      string // current directory of the active pane
	Command   string // foreground process in the active pane
	PID       int    // pane pid
	Created   time.Time
	Activity  time.Time
	Attached  bool
	Windows   int
	AgentDeck bool // true if the session carries our @agentdeck marker
	AgentHint string
}

const sep = "|~|"

// Available reports whether tmux is installed.
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return out.String(), nil
}

// List returns every tmux session on this machine.
func List() ([]Session, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{pane_current_path}",
		"#{pane_current_command}",
		"#{pane_pid}",
		"#{session_created}",
		"#{session_activity}",
		"#{window_activity}",
		"#{session_attached}",
		"#{session_windows}",
		"#{@agentdeck}",
		"#{@agentdeck_agent}",
	}, sep)
	out, err := run("list-sessions", "-F", format)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "No such file") {
			return nil, nil
		}
		return nil, err
	}
	return parseList(out), nil
}

// parseList turns the output of list-sessions -F into sessions. Lines that do
// not carry every field are skipped rather than guessed at.
//
// Activity merges two tmux clocks. session_activity only moves when the session
// is interacted with, so for the detached sessions agentdeck runs it stays
// pinned at creation time; window_activity is the one that advances as the pane
// produces output. Take whichever is later.
func parseList(out string) []Session {
	var sessions []Session
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, sep)
		if len(f) < 11 {
			continue
		}
		s := Session{Name: f[0], Path: f[1], Command: f[2]}
		s.PID, _ = strconv.Atoi(f[3])
		if ts, err := strconv.ParseInt(f[4], 10, 64); err == nil {
			s.Created = time.Unix(ts, 0)
		}
		s.Activity = laterOf(f[5], f[6])
		s.Attached = f[7] != "0" && f[7] != ""
		s.Windows, _ = strconv.Atoi(f[8])
		s.AgentDeck = f[9] == "1"
		s.AgentHint = f[10]
		sessions = append(sessions, s)
	}
	return sessions
}

// laterOf returns the more recent of two unix timestamps, ignoring ones tmux
// did not fill in.
func laterOf(a, b string) time.Time {
	var out time.Time
	for _, v := range []string{a, b} {
		ts, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		if t := time.Unix(ts, 0); t.After(out) {
			out = t
		}
	}
	return out
}

// New creates a detached session running the given command in dir.
func New(name, dir, agentID string, argv []string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("directory %q: %w", dir, err)
	}
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	if len(argv) > 0 {
		args = append(args, argv...)
	}
	if _, err := run(args...); err != nil {
		return err
	}
	_, _ = run("set-option", "-t", name, "@agentdeck", "1")
	_, _ = run("set-option", "-t", name, "@agentdeck_agent", agentID)
	return nil
}

// Capture returns the last n lines of the session's active pane.
func Capture(name string, n int) (string, error) {
	out, err := run("capture-pane", "-p", "-t", name, "-S", fmt.Sprintf("-%d", n))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// Send types text into the session followed by Enter.
func Send(name, text string) error {
	_, err := run("send-keys", "-t", name, text, "Enter")
	return err
}

// Kill destroys a session.
func Kill(name string) error {
	_, err := run("kill-session", "-t", name)
	return err
}

// AttachCmd returns the exec.Cmd to attach the current terminal to a session.
func AttachCmd(name string) *exec.Cmd {
	if os.Getenv("TMUX") != "" {
		return exec.Command("tmux", "switch-client", "-t", name)
	}
	return exec.Command("tmux", "attach-session", "-t", name)
}

// Exists reports whether a session with the name exists.
func Exists(name string) bool {
	_, err := run("has-session", "-t", "="+name)
	return err == nil
}
