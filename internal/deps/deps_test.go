package deps

import (
	"slices"
	"testing"
)

func TestMissingPackages(t *testing.T) {
	log := stringsJoin(
		"nasm: command not found",
		"bash: wasm-opt: command not found",
		"Package 'libudev' not found",
		"ld: cannot find -lssl",
		"Unable to find libclang",
		"sh: command not found",
	)
	got := MissingPackages(log)
	want := []string{"nasm", "binaryen", "systemd", "openssl", "clang"}
	if !slices.Equal(got, want) {
		t.Fatalf("packages %v, want %v", got, want)
	}
}

func TestMentionedCargo(t *testing.T) {
	got := Mentioned([]string{"bash", "-c", "cargo install --features nasm && wasm-opt -O2 a.wasm"})
	want := []string{"cargo", "nasm", "wasm-opt"}
	if !slices.Equal(got, want) {
		t.Fatalf("mentioned %v, want %v", got, want)
	}
}

func TestMergeEnvReplaces(t *testing.T) {
	got := MergeEnv([]string{"PATH=/usr/bin", "A=1"}, map[string]string{"PATH": "/opt/cargo/bin:/usr/bin"})
	if !slices.Equal(got, []string{"A=1", "PATH=/opt/cargo/bin:/usr/bin"}) {
		t.Fatalf("env %v", got)
	}
}

func stringsJoin(lines ...string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
