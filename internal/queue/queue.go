// Package queue schedules builds onto the machine's CPUs.
// Packages that install into the image stay in order. Other builds run
// beside each other when their thread counts fit.
package queue

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
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

	KindMake     = "make"
	KindMakepkg  = "makepkg"
	KindAur      = "aur"
	KindStrategy = "strategy"

	// PullArtifact is the tarball of paths a strategy asked to bring back.
	PullArtifact = "pull.tar.gz"

	// keepFinished is how many completed job directories stay on disk.
	// Each one holds a log and any built packages, not the build tree.
	keepFinished = 8
)

type Job struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	Jobs          int       `json:"jobs"`
	Position      int       `json:"position,omitempty"`
	Build         []string  `json:"build,omitempty"`
	Pull          []string  `json:"pull,omitempty"`
	KernelRelease string    `json:"kernelrelease,omitempty"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	Created       time.Time `json:"created"`
	Started       time.Time `json:"started,omitempty"`
	Finished      time.Time `json:"finished,omitempty"`
}

// Runner builds one job. dir is the unpacked tree, empty for an AUR clone the runner creates.
type Runner func(job *Job, dir string, logf func(string)) error

// HostInfo is how full the builder is.
type HostInfo struct {
	CPUs    int `json:"cpus"`
	Busy    int `json:"busy"`
	Running int `json:"running"`
	Queued  int `json:"queued"`
}

type Queue struct {
	root   string
	runner Runner
	cpus   int

	mu      sync.Mutex
	order   []string
	jobs    map[string]*Job
	wait    chan struct{}
	seq     atomic.Uint64
	busy    int
	holding bool
}

func New(root string, runner Runner) (*Queue, error) {
	return newQueue(root, runner, CPUCount(runtime.NumCPU(), os.Getenv("CO_CPUS")))
}

// CPUCount is the core budget. A CO_CPUS value between 1 and the machine
// size lowers it. Anything else leaves the machine size alone.
func CPUCount(detected int, raw string) int {
	if detected < 1 {
		detected = 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > detected {
		return detected
	}
	return n
}

func newQueue(root string, runner Runner, cpus int) (*Queue, error) {
	if cpus < 1 {
		cpus = 1
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if runner == nil {
		runner = DefaultRunner
	}
	q := &Queue{
		root:   root,
		runner: runner,
		cpus:   cpus,
		jobs:   map[string]*Job{},
		wait:   make(chan struct{}, 1),
	}
	q.sweepStale()
	go q.loop()
	return q, nil
}

func (q *Queue) Host() HostInfo {
	q.mu.Lock()
	defer q.mu.Unlock()
	h := HostInfo{CPUs: q.cpus, Busy: q.busy}
	for _, job := range q.jobs {
		switch job.Status {
		case StatusRunning:
			h.Running++
		case StatusQueued:
			h.Queued++
		}
	}
	return h
}

func (q *Queue) loop() {
	for {
		if !q.startNext() {
			<-q.wait
		}
	}
}

func (q *Queue) startNext() bool {
	q.mu.Lock()
	id := q.takeNext()
	if id == "" {
		q.mu.Unlock()
		return false
	}
	job := q.jobs[id]
	job.Status = StatusRunning
	job.Started = time.Now()
	q.busy += job.Jobs
	if exclusive(job.Kind) {
		q.holding = true
	}
	q.mu.Unlock()
	go q.execute(job)
	return true
}

// takeNext pulls the oldest job that fits. It will not step past a job
// that is waiting for cores or for the package install lock.
func (q *Queue) takeNext() string {
	if len(q.order) == 0 {
		return ""
	}
	id := q.order[0]
	job := q.jobs[id]
	if job == nil {
		q.order = q.order[1:]
		return q.takeNext()
	}
	if q.busy > 0 && q.busy+job.Jobs > q.cpus {
		return ""
	}
	if exclusive(job.Kind) && q.holding {
		return ""
	}
	q.order = q.order[1:]
	return id
}

func exclusive(kind string) bool {
	return kind == KindAur || kind == KindMakepkg
}

func (q *Queue) execute(job *Job) {
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
	var release string
	if err == nil && job.Kind == KindStrategy {
		release = readKernelRelease(dir)
		if len(job.Pull) > 0 {
			if perr := q.stagePull(job); perr != nil {
				err = perr
			}
		}
	}
	if cerr := q.releaseWork(job.ID); cerr != nil {
		logf("cleanup: " + cerr.Error())
	}
	q.mu.Lock()
	job.KernelRelease = release
	job.Finished = time.Now()
	q.busy -= job.Jobs
	if q.busy < 0 {
		q.busy = 0
	}
	if exclusive(job.Kind) {
		q.holding = false
	}
	if err != nil {
		job.Status = StatusFailed
		job.Error = err.Error()
		logf(err.Error())
	} else {
		job.Status = StatusSucceeded
	}
	q.mu.Unlock()
	q.pruneFinished()
	q.wake()
}

func (q *Queue) wake() {
	select {
	case q.wait <- struct{}{}:
	default:
	}
}

// SubmitAur enqueues a package the runner will clone from the AUR.
func (q *Queue) SubmitAur(name string) *Job {
	job := q.newJob(KindAur, name, 0)
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

// SubmitStrategy unpacks the tree, then enqueues the build commands.
func (q *Queue) SubmitStrategy(name string, jobs int, build, pull []string, tarGz []byte) (*Job, error) {
	if len(build) == 0 {
		return nil, fmt.Errorf("strategy has no build commands")
	}
	for _, p := range pull {
		if _, err := cleanRel(p); err != nil {
			return nil, err
		}
	}
	job := q.newJob(KindStrategy, name, jobs)
	job.Build = build
	job.Pull = append([]string(nil), pull...)
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
	if q.cpus < 1 {
		q.cpus = 1
	}
	if jobs <= 0 || jobs > q.cpus {
		jobs = q.cpus
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
	q.wake()
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
	pos := map[string]int{}
	for i, id := range q.order {
		pos[id] = i + 1
	}
	var out []*Job
	for _, job := range q.jobs {
		cp := *job
		cp.Position = pos[job.ID]
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

func (q *Queue) artifactDir(id string) string {
	return filepath.Join(q.root, id, "artifacts")
}

// ArtifactPaths lists built packages for a job. After cleanup they live
// beside the log, not in the build tree.
func (q *Queue) ArtifactPaths(id string) ([]string, error) {
	art := q.artifactDir(id)
	if st, err := os.Stat(art); err == nil && st.IsDir() {
		return Packages(art)
	}
	return Packages(q.SrcDir(id))
}

// ArtifactFile returns the package path, or an empty string when it is not there.
func (q *Queue) ArtifactFile(id, name string) string {
	art := filepath.Join(q.artifactDir(id), name)
	if st, err := os.Stat(art); err == nil && !st.IsDir() {
		return art
	}
	src := filepath.Join(q.SrcDir(id), name)
	if st, err := os.Stat(src); err == nil && !st.IsDir() {
		return src
	}
	return ""
}

// releaseWork moves built packages next to the log and deletes the build tree.
func (q *Queue) releaseWork(id string) error {
	src := q.SrcDir(id)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pkgs, err := Packages(src)
	if err != nil {
		return err
	}
	if len(pkgs) > 0 {
		art := q.artifactDir(id)
		if err := os.MkdirAll(art, 0o755); err != nil {
			return err
		}
		for _, p := range pkgs {
			if err := os.Rename(p, filepath.Join(art, filepath.Base(p))); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(src)
}

func cleanRel(p string) (string, error) {
	if p == "" || filepath.IsAbs(p) {
		return "", fmt.Errorf("bad pull path %q", p)
	}
	c := filepath.Clean(p)
	if c == ".." || strings.HasPrefix(c, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("bad pull path %q", p)
	}
	return c, nil
}

func readKernelRelease(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err != nil {
		return ""
	}
	cmd := exec.Command("make", "-s", "kernelrelease")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// stagePull packs the strategy's pull paths next to the log, before the tree is deleted.
func (q *Queue) stagePull(job *Job) error {
	src := q.SrcDir(job.ID)
	if err := os.MkdirAll(q.artifactDir(job.ID), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(q.artifactDir(job.ID), PullArtifact))
	if err != nil {
		return err
	}
	var rels []string
	for _, p := range job.Pull {
		c, err := cleanRel(p)
		if err != nil {
			f.Close()
			return err
		}
		rels = append(rels, c)
	}
	packErr := archive.PackPaths(src, rels, f)
	closeErr := f.Close()
	if packErr != nil {
		return packErr
	}
	return closeErr
}

// sweepStale drops build trees left behind by a crashed process, then
// trims old job directories.
func (q *Queue) sweepStale() {
	entries, err := os.ReadDir(q.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_ = os.RemoveAll(filepath.Join(q.root, e.Name(), "src"))
	}
	q.pruneFinished()
}

func (q *Queue) pruneFinished() {
	active := map[string]bool{}
	q.mu.Lock()
	for id, job := range q.jobs {
		if job.Status == StatusQueued || job.Status == StatusRunning {
			active[id] = true
		}
	}
	q.mu.Unlock()

	entries, err := os.ReadDir(q.root)
	if err != nil {
		return
	}
	type item struct {
		name string
		mod  time.Time
	}
	var finished []item
	for _, e := range entries {
		if !e.IsDir() || active[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		finished = append(finished, item{e.Name(), info.ModTime()})
	}
	sort.Slice(finished, func(i, j int) bool {
		return finished[i].mod.After(finished[j].mod)
	})
	if len(finished) <= keepFinished {
		return
	}
	for _, it := range finished[keepFinished:] {
		_ = os.RemoveAll(filepath.Join(q.root, it.name))
		q.mu.Lock()
		delete(q.jobs, it.name)
		q.mu.Unlock()
	}
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
		return runMakepkg(dir, logf, job.Jobs)
	case KindMakepkg:
		return runMakepkg(dir, logf, job.Jobs)
	case KindMake:
		cmd := exec.Command("make", fmt.Sprintf("-j%d", job.Jobs))
		cmd.Dir = dir
		cmd.Env = buildEnv(job.Jobs)
		return runLogged(cmd, logf)
	case KindStrategy:
		for _, line := range job.Build {
			cmd := exec.Command("bash", "-c", line)
			cmd.Dir = dir
			cmd.Env = buildEnv(job.Jobs)
			if err := runLogged(cmd, logf); err != nil {
				return fmt.Errorf("%s: %w", line, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown kind %s", job.Kind)
	}
}

func buildEnv(jobs int) []string {
	n := fmt.Sprintf("%d", jobs)
	return append(os.Environ(),
		"CO_JOBS="+n,
		"MAKEFLAGS=-j"+n,
		"NINJAFLAGS=-j"+n,
		"CMAKE_BUILD_PARALLEL_LEVEL="+n,
		"CCACHE_DIR=/var/cache/ccache",
		"CCACHE_MAXSIZE=20G",
	)
}

func runMakepkg(dir string, logf func(string), jobs int) error {
	n := fmt.Sprintf("%d", jobs)
	_ = exec.Command("chown", "-R", "builder:builder", dir).Run()
	cmd := exec.Command("su", "-s", "/bin/bash", "builder", "-c",
		"export CO_JOBS="+n+" MAKEFLAGS=-j"+n+" NINJAFLAGS=-j"+n+" CMAKE_BUILD_PARALLEL_LEVEL="+n+" CCACHE_DIR=/var/cache/ccache CCACHE_MAXSIZE=20G; makepkg -f --noconfirm --syncdeps")
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
