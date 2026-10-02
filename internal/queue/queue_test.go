package queue

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
)

func TestSerialOrder(t *testing.T) {
	dir := t.TempDir()
	var ran []string
	done := make(chan struct{})
	q, err := New(dir, func(job *Job, _ string, _ func(string)) error {
		ran = append(ran, job.Name)
		if len(ran) == 2 {
			close(done)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q.SubmitAur("first")
	q.SubmitAur("second")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if ran[0] != "first" || ran[1] != "second" {
		t.Fatalf("order %v", ran)
	}
}

func TestSubmitTreeBeforeRun(t *testing.T) {
	dir := t.TempDir()
	started := make(chan struct{})
	q, err := New(dir, func(job *Job, src string, _ func(string)) error {
		if _, err := os.Stat(filepath.Join(src, "PKGBUILD")); err != nil {
			t.Errorf("tree missing at start: %v", err)
		}
		close(started)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "PKGBUILD"), []byte("pkgname=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := archive.Pack(src, &buf); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SubmitTree(KindMakepkg, "pkg", 1, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
