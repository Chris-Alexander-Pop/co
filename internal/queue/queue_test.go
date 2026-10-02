package queue

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestReleaseKeepsPackageDropsTree(t *testing.T) {
	root := t.TempDir()
	q, err := New(root, func(job *Job, src string, _ func(string)) error {
		if err := os.MkdirAll(filepath.Join(src, "src"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(src, "src", "big.o"), []byte("obj"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(src, "demo.pkg.tar.zst"), []byte("pkg"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	job := q.SubmitAur("demo")
	waitStatus(t, q, job.ID, StatusSucceeded)
	if _, err := os.Stat(q.SrcDir(job.ID)); !os.IsNotExist(err) {
		t.Fatalf("build tree still present: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, job.ID, "artifacts", "demo.pkg.tar.zst"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pkg" {
		t.Fatalf("package %q", got)
	}
}

func TestPruneKeepsQueuedTree(t *testing.T) {
	root := t.TempDir()
	q, err := New(root, func(job *Job, _ string, _ func(string)) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q.mu.Lock()
	q.jobs["queued"] = &Job{ID: "queued", Status: StatusQueued}
	q.mu.Unlock()
	src := filepath.Join(root, "queued", "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.c"), []byte("int x;"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < keepFinished+2; i++ {
		name := filepath.Join(root, fmt.Sprintf("old-%d", i))
		if err := os.MkdirAll(name, 0o755); err != nil {
			t.Fatal(err)
		}
		when := old.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(name, when, when); err != nil {
			t.Fatal(err)
		}
	}
	q.pruneFinished()
	if _, err := os.Stat(filepath.Join(src, "file.c")); err != nil {
		t.Fatal(err)
	}
	left := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[:4] == "old-" {
			left++
		}
	}
	if left != keepFinished {
		t.Fatalf("kept %d old jobs", left)
	}
}

func TestStagePullThenDropSrc(t *testing.T) {
	root := t.TempDir()
	q, err := New(root, func(job *Job, _ string, _ func(string)) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id := "9"
	src := q.SrcDir(id)
	if err := os.MkdirAll(filepath.Join(src, "out", "boot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "out", "boot", "vmlinuz"), []byte("kern"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "junk.o"), []byte("obj"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := &Job{ID: id, Pull: []string{"out"}}
	if err := q.stagePull(job); err != nil {
		t.Fatal(err)
	}
	if err := q.releaseWork(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src still present: %v", err)
	}
	f, err := os.Open(filepath.Join(q.artifactDir(id), PullArtifact))
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := archive.Unpack(f, dest); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	got, err := os.ReadFile(filepath.Join(dest, "out", "boot", "vmlinuz"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "kern" {
		t.Fatalf("payload %q", got)
	}
}

func TestRejectEscapingPull(t *testing.T) {
	root := t.TempDir()
	q, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.SubmitStrategy("x", 1, []string{"true"}, []string{"../etc"}, []byte{}); err == nil {
		t.Fatal("expected bad pull path")
	}
}

func TestCPUCount(t *testing.T) {
	if got := CPUCount(16, "14"); got != 14 {
		t.Fatalf("cap %d", got)
	}
	if got := CPUCount(16, ""); got != 16 {
		t.Fatalf("empty %d", got)
	}
	if got := CPUCount(16, "32"); got != 16 {
		t.Fatalf("too high %d", got)
	}
	if got := CPUCount(8, "14"); got != 8 {
		t.Fatalf("above machine %d", got)
	}
	if got := CPUCount(16, "nope"); got != 16 {
		t.Fatalf("bad %d", got)
	}
}

func TestParallelWhenTheyFit(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)
	q, err := newQueue(t.TempDir(), func(job *Job, _ string, _ func(string)) error {
		started <- job.Name
		<-release
		return nil
	}, 4)
	if err != nil {
		t.Fatal(err)
	}
	q.push(q.newJob(KindMake, "a", 2))
	q.push(q.newJob(KindMake, "b", 2))
	got := map[string]bool{}
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("started %v", got)
		}
	}
}

func TestDoesNotSkipAJobThatDoesNotFit(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)
	q, err := newQueue(t.TempDir(), func(job *Job, _ string, _ func(string)) error {
		started <- job.Name
		<-release
		return nil
	}, 4)
	if err != nil {
		t.Fatal(err)
	}
	q.push(q.newJob(KindMake, "big", 4))
	q.push(q.newJob(KindMake, "small", 1))
	select {
	case name := <-started:
		if name != "big" {
			t.Fatalf("started %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("big never started")
	}
	select {
	case name := <-started:
		t.Fatalf("started %s while big holds the cores", name)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAurDoesNotOverlap(t *testing.T) {
	var mu sync.Mutex
	var n int
	q, err := newQueue(t.TempDir(), func(job *Job, _ string, _ func(string)) error {
		mu.Lock()
		n++
		if n > 1 {
			t.Errorf("overlap on %s", job.Name)
		}
		mu.Unlock()
		time.Sleep(40 * time.Millisecond)
		mu.Lock()
		n--
		mu.Unlock()
		return nil
	}, 8)
	if err != nil {
		t.Fatal(err)
	}
	q.SubmitAur("a")
	q.SubmitAur("b")
	deadline := time.Now().Add(3 * time.Second)
	for {
		done := 0
		for _, job := range q.List() {
			if job.Status == StatusSucceeded || job.Status == StatusFailed {
				done++
			}
		}
		if done == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func waitStatus(t *testing.T, q *Queue, id, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		job, ok := q.Get(id)
		if ok && job.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s status timeout, last ok=%v", id, ok)
		}
		time.Sleep(15 * time.Millisecond)
	}
}
