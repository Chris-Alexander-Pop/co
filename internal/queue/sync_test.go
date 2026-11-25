package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncReposUpgrades(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > '" + logPath + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := syncRepos(func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "-Syu --noconfirm" {
		t.Fatalf("pacman args %q", got)
	}
}

func TestSyncReposFailure(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'database is locked' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var logged strings.Builder
	err := syncRepos(func(line string) { logged.WriteString(line) })
	if err == nil {
		t.Fatal("expected pacman failure")
	}
	if !strings.Contains(logged.String(), "database is locked") {
		t.Fatalf("log %q", logged.String())
	}
}
