// Package store persists metadata about sessions agentdeck created.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Meta is what we remember about a session beyond what tmux knows.
type Meta struct {
	Name      string    `json:"name"`
	Agent     string    `json:"agent"`
	Dir       string    `json:"dir"`
	Prompt    string    `json:"prompt,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type file struct {
	Sessions map[string]Meta `json:"sessions"`
}

var mu sync.Mutex

func path() string {
	if p := os.Getenv("AGENTDECK_HOME"); p != "" {
		return filepath.Join(p, "sessions.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentdeck", "sessions.json")
}

func load() (file, error) {
	var f file
	f.Sessions = map[string]Meta{}
	b, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	if f.Sessions == nil {
		f.Sessions = map[string]Meta{}
	}
	return f, nil
}

func save(f file) error {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// All returns all stored metadata keyed by session name.
func All() (map[string]Meta, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	return f.Sessions, err
}

// Put stores metadata for a session.
func Put(m Meta) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return err
	}
	f.Sessions[m.Name] = m
	return save(f)
}

// Delete forgets a session.
func Delete(name string) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return err
	}
	delete(f.Sessions, name)
	return save(f)
}
