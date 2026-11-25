package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
)

func TestFlagBuildOpts(t *testing.T) {
	seed, bg, rest, err := flagBuildOpts([]string{"--seed-cache", "--no-install"})
	if err != nil || !seed || bg || len(rest) != 1 || rest[0] != "--no-install" {
		t.Fatalf("seed=%v bg=%v rest=%v err=%v", seed, bg, rest, err)
	}
	_, _, _, err = flagBuildOpts([]string{"--seed-cache", "--bg"})
	if err == nil {
		t.Fatal("expected conflict with --bg")
	}
	seed, bg, rest, err = flagBuildOpts([]string{"--bg"})
	if err != nil || seed || !bg || len(rest) != 0 {
		t.Fatalf("bg only: seed=%v bg=%v rest=%v err=%v", seed, bg, rest, err)
	}
}

func TestPackCargoRoundTrip(t *testing.T) {
	root := t.TempDir()
	cargo := filepath.Join(root, "cargo")
	reg := filepath.Join(cargo, "registry", "src", "crate")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reg, "lib.rs"), []byte("fn x() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cargo, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cargo, "bin", "tool"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	blob, err := packCargo(cargo)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) == 0 {
		t.Fatal("expected cargo archive")
	}
	dest := t.TempDir()
	if err := archive.Unpack(bytes.NewReader(blob), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "registry", "src", "crate", "lib.rs")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "bin", "tool")); !os.IsNotExist(err) {
		t.Fatal("bin should not be packed")
	}
}

func TestPackIfPresent(t *testing.T) {
	empty := t.TempDir()
	blob, err := packIfPresent(empty)
	if err != nil || blob != nil {
		t.Fatalf("empty dir: %v %v", blob, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "obj"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	blob, err = packIfPresent(dir)
	if err != nil || len(blob) == 0 {
		t.Fatalf("pack: %v %v", len(blob), err)
	}
	dest := t.TempDir()
	if err := archive.Unpack(bytes.NewReader(blob), dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "obj"))
	if err != nil || string(got) != "cache" {
		t.Fatalf("round trip: %q %v", got, err)
	}
}
