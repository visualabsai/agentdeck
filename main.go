// agentdeck — one dashboard for all your CLI coding agents.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/visualabsai/agentdeck/internal/agent"
	"github.com/visualabsai/agentdeck/internal/session"
	"github.com/visualabsai/agentdeck/internal/tmux"
	"github.com/visualabsai/agentdeck/internal/ui"
)

const usage = `agentdeck — manage Claude Code, Codex, Gemini and local-model sessions

usage:
  agentdeck                              open the dashboard
  agentdeck new <agent> [dir] [-- prompt] start an agent (claude|codex|gemini|opencode|llama)
  agentdeck ls                           list sessions
  agentdeck attach <name>                attach to a session
  agentdeck send <name> <text>           type text into a session
  agentdeck kill <name>                  kill a session
  agentdeck agents                       list known agents
  agentdeck version                      print the version
`

// version is stamped into release builds with -ldflags. Binaries produced by
// "go install" carry no ldflags, so fall back to the module version the Go
// toolchain records in the build info.
var version = "dev"

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}

func main() {
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		requireTmux()
		runTUI()
		return
	case args[0] == "-h", args[0] == "--help", args[0] == "help":
		fmt.Print(usage)
		return
	case args[0] == "version", args[0] == "--version", args[0] == "-v":
		fmt.Println("agentdeck", buildVersion())
		return
	case args[0] == "agents":
		for _, a := range agent.All() {
			fmt.Printf("%-9s %-22s %s %s\n", a.ID, a.Name, a.Command, strings.Join(a.Args, " "))
		}
		return
	}
	// Everything below drives tmux.
	requireTmux()
	switch args[0] {
	case "new", "n":
		cmdNew(args[1:])
	case "ls", "list":
		cmdList()
	case "attach", "a":
		need(args, 2, "attach <name>")
		exit(runAttach(args[1]))
	case "send":
		need(args, 3, "send <name> <text>")
		exit(tmux.Send(args[1], strings.Join(args[2:], " ")))
	case "kill", "rm":
		need(args, 2, "kill <name>")
		exit(session.Kill(args[1]))
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func runTUI() {
	p := tea.NewProgram(ui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdNew(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentdeck new <agent> [dir] [-- prompt]")
		os.Exit(2)
	}
	agentID := args[0]
	dir, prompt := "", ""
	rest := args[1:]
	for i, a := range rest {
		if a == "--" {
			prompt = strings.Join(rest[i+1:], " ")
			rest = rest[:i]
			break
		}
	}
	if len(rest) > 0 {
		dir = rest[0]
	}
	name, err := session.Launch(agentID, dir, "", prompt)
	if err != nil {
		exit(err)
	}
	fmt.Println(name)
}

func cmdList() {
	sessions, err := session.Discover()
	if err != nil {
		exit(err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGENT\tSTATUS\tDIR\tACTIVITY")
	for _, s := range sessions {
		a := s.Process
		if s.Agent != nil {
			a = s.Agent.ID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, a, s.Status, s.Dir, s.Activity.Format("15:04"))
	}
	w.Flush()
}

func runAttach(name string) error {
	c := tmux.AttachCmd(name)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func requireTmux() {
	if !tmux.Available() {
		fmt.Fprintln(os.Stderr, "agentdeck needs tmux: brew install tmux")
		os.Exit(1)
	}
}

func need(args []string, n int, u string) {
	if len(args) < n {
		fmt.Fprintln(os.Stderr, "usage: agentdeck "+u)
		os.Exit(2)
	}
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
