package store

import (
	"os"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	t.Setenv("AGENTDECK_HOME", t.TempDir())

	all, err := All()
	if err != nil {
		t.Fatalf("All on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("empty store returned %d entries", len(all))
	}

	m := Meta{Name: "claude-api", Agent: "claude", Dir: "/code/api", Prompt: "fix the test", CreatedAt: time.Now().Truncate(time.Second)}
	if err := Put(m); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := Put(Meta{Name: "codex-web", Agent: "codex", Dir: "/code/web"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	all, err = All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d entries, want 2", len(all))
	}
	got := all["claude-api"]
	if got.Agent != "claude" || got.Dir != "/code/api" || got.Prompt != "fix the test" {
		t.Errorf("round-trip lost data: %+v", got)
	}
	if !got.CreatedAt.Equal(m.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, m.CreatedAt)
	}

	if err := Delete("claude-api"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ = All()
	if _, ok := all["claude-api"]; ok {
		t.Error("Delete did not remove the entry")
	}
	if _, ok := all["codex-web"]; !ok {
		t.Error("Delete removed the wrong entry")
	}
}

func TestPutOverwritesSameName(t *testing.T) {
	t.Setenv("AGENTDECK_HOME", t.TempDir())
	_ = Put(Meta{Name: "s", Agent: "claude", Dir: "/a"})
	_ = Put(Meta{Name: "s", Agent: "codex", Dir: "/b"})
	all, _ := All()
	if len(all) != 1 || all["s"].Agent != "codex" || all["s"].Dir != "/b" {
		t.Fatalf("expected the second Put to win, got %+v", all)
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	t.Setenv("AGENTDECK_HOME", t.TempDir())
	if err := Delete("never-existed"); err != nil {
		t.Fatalf("Delete of unknown session: %v", err)
	}
}

func TestCorruptFileSurfacesError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTDECK_HOME", dir)
	if err := os.WriteFile(dir+"/sessions.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := All(); err == nil {
		t.Error("All should report a corrupt sessions.json")
	}
}
