// Package queue is a one-at-a-time build queue. Jobs run in submit order.
package queue

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"

	KindMake    = "make"
	KindMakepkg = "makepkg"
	KindAur     = "aur"
)

type Job struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name"`
	Jobs     int       `json:"jobs"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
}

// Runner builds one job. dir is the unpacked tree, empty for an AUR clone the runner creates.
type Runner func(job *Job, dir string, logf func(string)) error

type Queue struct {
	root   string
	runner Runner

	mu    sync.Mutex
	order []string
	jobs  map[string]*Job
	wait  chan struct{}
	seq   atomic.Uint64
}

func New(root string, runner Runner) (*Queue, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if runner == nil {
		runner = DefaultRunner
	}
	q := &Queue{
		root:   root,
		runner: runner,
		jobs:   map[string]*Job{},
		wait:   make(chan struct{}, 1),
	}
	go q.loop()
	return q, nil
}

func (q *Queue) loop() {
	for {
		q.mu.Lock()
		var id string
		if len(q.order) > 0 {
			id = q.order[0]
			q.order = q.order[1:]
		}
		job := q.jobs[id]
		if job != nil {
			job.Status = StatusRunning
			job.Started = time.Now()
		}
		q.mu.Unlock()
		if job == nil {
			<-q.wait
			continue
		}
		dir := filepath.Join(q.root, job.ID, "src")
		logPath := filepath.Join(q.root, job.ID, "log")
		logf := func(line string) {
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return
			}
			fmt.Fprintln(f, line)
			f.Close()
		}
		err := q.runner(job, dir, logf)
		q.mu.Lock()
		job.Finished = time.Now()
		if err != nil {
			job.Status = StatusFailed
			job.Error = err.Error()
			logf(err.Error())
		} else {
			job.Status = StatusSucceeded
		}
		q.mu.Unlock()
	}
}

// SubmitAur enqueues a package the runner will clone from the AUR.
func (q *Queue) SubmitAur(name string) *Job {
	job := q.newJob(KindAur, name, 1)
	q.push(job)
	return job
}

// SubmitTree unpacks the archive, then enqueues the job so the worker cannot start early.
func (q *Queue) SubmitTree(kind, name string, jobs int, tarGz []byte) (*Job, error) {
	if kind != KindMake && kind != KindMakepkg {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	job := q.newJob(kind, name, jobs)
	dir := filepath.Join(q.root, job.ID, "src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := archive.Unpack(bytes.NewReader(tarGz), dir); err != nil {
		return nil, err
	}
	q.push(job)
	return job, nil
}

func (q *Queue) newJob(kind, name string, jobs int) *Job {
	if jobs <= 0 {
		jobs = 1
	}
	return &Job{
		ID:      fmt.Sprintf("%d", q.seq.Add(1)),
		Kind:    kind,
		Name:    name,
		Jobs:    jobs,
		Status:  StatusQueued,
		Created: time.Now(),
	}
}

func (q *Queue) push(job *Job) {
	q.mu.Lock()
	q.jobs[job.ID] = job
	q.order = append(q.order, job.ID)
	q.mu.Unlock()
	os.MkdirAll(filepath.Join(q.root, job.ID), 0o755)
	select {
	case q.wait <- struct{}{}:
	default:
	}
}

func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	job, ok := q.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *job
	return &cp, true
}

func (q *Queue) List() []*Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*Job
	for _, job := range q.jobs {
		cp := *job
		out = append(out, &cp)
	}
	return out
}

func (q *Queue) LogPath(id string) string {
	return filepath.Join(q.root, id, "log")
}

func (q *Queue) SrcDir(id string) string {
	return filepath.Join(q.root, id, "src")
}

// DefaultRunner runs make or makepkg. AUR jobs are cloned into dir first.
func DefaultRunner(job *Job, dir string, logf func(string)) error {
	switch job.Kind {
	case KindAur:
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		cmd := exec.Command("git", "clone", "--depth", "1", "https://aur.archlinux.org/"+job.Name+".git", dir)
		if err := runLogged(cmd, logf); err != nil {
			return fmt.Errorf("clone %s: %w", job.Name, err)
		}
		return runMakepkg(dir, logf)
	case KindMakepkg:
		return runMakepkg(dir, logf)
	case KindMake:
		cmd := exec.Command("make", fmt.Sprintf("-j%d", job.Jobs))
		cmd.Dir = dir
		return runLogged(cmd, logf)
	default:
		return fmt.Errorf("unknown kind %s", job.Kind)
	}
}

func runMakepkg(dir string, logf func(string)) error {
	_ = exec.Command("chown", "-R", "builder:builder", dir).Run()
	cmd := exec.Command("su", "-s", "/bin/bash", "builder", "-c", "makepkg -f --noconfirm --syncdeps")
	cmd.Dir = dir
	if err := runLogged(cmd, logf); err != nil {
		return err
	}
	pkgs, err := Packages(dir)
	if err != nil || len(pkgs) == 0 {
		return err
	}
	args := append([]string{"-U", "--noconfirm"}, pkgs...)
	install := exec.Command("pacman", args...)
	install.Dir = dir
	return runLogged(install, logf)
}

func runLogged(cmd *exec.Cmd, logf func(string)) error {
	r, w := io.Pipe()
	cmd.Stdout = w
	cmd.Stderr = w
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				logf(string(buf[:n]))
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()
	err := cmd.Run()
	w.Close()
	<-done
	return err
}

// Packages lists built package tarballs under dir.
func Packages(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.pkg.tar.*"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range matches {
		if filepath.Base(m) == "" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
