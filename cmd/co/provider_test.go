package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLib32TwinFollowsPipewire(t *testing.T) {
	provides := map[string][]string{
		"pipewire-jack": {"jack", "libjack.so"},
		"alsa-lib":      nil,
	}
	if got := lib32Twin("lib32-jack", provides); got != "lib32-pipewire-jack" {
		t.Fatalf("jack twin %q", got)
	}
	if got := lib32Twin("lib32-alsa-lib", provides); got != "" {
		t.Fatalf("concrete dep returned %q", got)
	}
	if got := lib32Twin("glibc", provides); got != "" {
		t.Fatalf("non-lib32 returned %q", got)
	}
}

func TestParseDepends(t *testing.T) {
	info := "pkgname = lib32-portaudio\ndepend = lib32-gcc-libs\ndepend = lib32-jack\ndepend = libjack.so\n"
	got := parseDepends(info)
	if len(got) != 3 || got[1] != "lib32-jack" {
		t.Fatalf("%v", got)
	}
}

func TestReadProvides(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "pipewire-jack-1.6.9-1")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "%NAME%\npipewire-jack\n\n%PROVIDES%\njack\nlibjack.so=0-64\n"
	if err := os.WriteFile(filepath.Join(dir, "desc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readProvides(root)
	if err != nil {
		t.Fatal(err)
	}
	twin := lib32Twin("lib32-jack", got)
	if twin != "lib32-pipewire-jack" {
		t.Fatalf("twin %q from %+v", twin, got)
	}
}
