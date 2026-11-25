package aur

import (
	"reflect"
	"testing"
)

func TestParseQua(t *testing.T) {
	got := ParseQua("foo 1-1 -> 2-1\n\nbar 3-1 -> 4-1\n")
	if len(got) != 2 || got[0] != "foo" || got[1] != "bar" {
		t.Fatalf("%v", got)
	}
}

func TestParseUpgrades(t *testing.T) {
	got := ParseUpgrades("topgrade 17.9.0-1 -> 17.12.3-1\nlibudev0-shim 1-1 -> 2-2\n")
	if len(got) != 2 || got[0].Name != "topgrade" || got[0].New != "17.12.3-1" || got[1].New != "2-2" {
		t.Fatalf("%+v", got)
	}
}

func TestArtifactHasVersion(t *testing.T) {
	if !ArtifactHasVersion("topgrade", "17.12.3-1", "topgrade-17.12.3-1-x86_64.pkg.tar.zst") {
		t.Fatal("expected match")
	}
	if ArtifactHasVersion("topgrade", "17.12.3-1", "topgrade-17.12.30-1-x86_64.pkg.tar.zst") {
		t.Fatal("longer version matched")
	}
	if ArtifactHasVersion("topgrade", "17.12.3-1", "topgrade-debug-17.12.3-1-x86_64.pkg.tar.zst") {
		t.Fatal("debug package matched the base name")
	}
	if !ArtifactHasVersion("jlink-software-and-documentation", "66:9.82-1", "jlink-software-and-documentation-66:9.82-1-x86_64.pkg.tar.zst") {
		t.Fatal("epoch")
	}
}

func TestMissingDeps(t *testing.T) {
	log := `
warning: cannot resolve "unionfs-fuse", a dependency of "flutter-bin"
:: unable to satisfy dependency 'unionfs-fuse' required by flutter-bin
unable to satisfy dependency 'libfoo' required by bar
`
	got := MissingDeps(log)
	want := []string{"unionfs-fuse", "libfoo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
