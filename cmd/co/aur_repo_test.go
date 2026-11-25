package main

import (
	"os/exec"
	"testing"
)

func TestRepoSatisfiesProvide(t *testing.T) {
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman")
	}
	if !pacmanQuiet("-T", "sdl2") && !pacmanQuiet("-Sp", "--print-format", "%n", "sdl2") {
		t.Skip("no sdl2 provider")
	}
	if !repoSatisfies("sdl2") {
		t.Fatal("sdl2")
	}
	if repoSatisfies("co-no-such-dep-zz") {
		t.Fatal("missing name treated as provided")
	}
}
