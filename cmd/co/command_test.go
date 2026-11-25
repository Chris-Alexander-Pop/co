package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	flags, cmd, ok := splitCommand([]string{"--bg", "--fallback", "cancel", "--command", "cargo", "build", "--release"})
	if !ok {
		t.Fatal("expected command")
	}
	if !slices.Equal(flags, []string{"--bg", "--fallback", "cancel"}) {
		t.Fatalf("flags %v", flags)
	}
	if !slices.Equal(cmd, []string{"cargo", "build", "--release"}) {
		t.Fatalf("cmd %v", cmd)
	}
	if _, _, ok := splitCommand([]string{"status"}); ok {
		t.Fatal("status is not --command")
	}
	_, cmd, ok = splitCommand([]string{"--command"})
	if !ok || len(cmd) != 0 {
		t.Fatalf("empty command ok=%v cmd=%v", ok, cmd)
	}
}

func TestSkipHeavy(t *testing.T) {
	if !skipHeavy("target", true) || !skipHeavy(filepath.Join("a", "node_modules"), true) {
		t.Fatal("build dirs should skip")
	}
	if skipHeavy("src/main.go", false) || skipHeavy("src", true) {
		t.Fatal("source should upload")
	}
}
