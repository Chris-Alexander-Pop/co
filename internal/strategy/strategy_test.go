package strategy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindByDir(t *testing.T) {
	projects := t.TempDir()
	cwd := t.TempDir()
	body := "dir: " + cwd + "\nbuild:\n  - make -j{jobs}\npull:\n  - out\ninstall:\n  - sudo make install\n"
	if err := os.WriteFile(filepath.Join(projects, "other.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Find(projects, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Build[0] != "make -j{jobs}" || got.Pull[0] != "out" {
		t.Fatalf("got %#v", got)
	}
}

func TestFindMiss(t *testing.T) {
	projects := t.TempDir()
	got, err := Find(projects, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got %#v", got)
	}
}

func TestExpand(t *testing.T) {
	got := Expand([]string{"make -j{jobs}", "img-{kernelrelease}"}, 16, "7.2.6-zen2")
	if got[0] != "make -j16" || got[1] != "img-7.2.6-zen2" {
		t.Fatalf("%v", got)
	}
}
