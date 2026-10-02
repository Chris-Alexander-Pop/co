package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/history"
	"github.com/Chris-Alexander-Pop/co/internal/progress"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

type session struct {
	track    map[string]*progress.Tracker
	have     map[string]string
	rate     map[string]*progress.Rate
	cpusHint int
	book     *history.Book
	noted    map[string]bool
}

func newSession(cpusHint int) *session {
	return &session{
		track:    map[string]*progress.Tracker{},
		have:     map[string]string{},
		rate:     map[string]*progress.Rate{},
		cpusHint: cpusHint,
	}
}

func follow(cli *client.Client, id string) (*queue.Job, error) {
	s := newSession(cpuHint())
	var painted int
	var last string
	for {
		block, jobs, err := s.render(cli)
		if err != nil {
			return nil, err
		}
		job := findJob(jobs, id)
		if job == nil {
			return nil, fmt.Errorf("job %s is gone", id)
		}
		if ui.Enabled {
			painted = ui.Paint(os.Stdout, painted, block)
		} else if block != last || job.Status == queue.StatusSucceeded || job.Status == queue.StatusFailed {
			fmt.Print(block)
			last = block
		}
		switch job.Status {
		case queue.StatusSucceeded:
			return job, nil
		case queue.StatusFailed:
			if tail := logTail(s.have[id], 12); strings.TrimSpace(tail) != "" {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, tail)
			}
			if job.Error != "" {
				return job, fmt.Errorf("%s", job.Error)
			}
			return job, fmt.Errorf("job %s failed", id)
		}
		time.Sleep(time.Second)
	}
}

func (s *session) render(cli *client.Client) (string, []queue.Job, error) {
	jobs, err := cli.List()
	if err != nil {
		return "", nil, err
	}
	sortJobs(jobs)
	s.syncHistory(cli, jobs)
	host, herr := cli.Host()
	if herr != nil {
		host = queue.HostInfo{}
	}
	var lanes []ui.Lane
	runningN, queuedN, busy := 0, 0, 0
	for i := range jobs {
		switch jobs[i].Status {
		case queue.StatusRunning:
			runningN++
			busy += jobs[i].Jobs
		case queue.StatusQueued:
			queuedN++
		}
	}
	if host.CPUs == 0 {
		host.Busy = busy
		host.Running = runningN
		host.Queued = queuedN
		if s.cpusHint > host.Busy {
			host.CPUs = s.cpusHint
		} else if host.Busy > 0 {
			host.CPUs = host.Busy
		}
	}
	place := 0
	for i := range jobs {
		job := &jobs[i]
		lane := ui.Lane{
			Name:     displayName(job),
			Status:   job.Status,
			Threads:  job.Jobs,
			Elapsed:  elapsed(job),
			Position: job.Position,
		}
		switch job.Status {
		case queue.StatusRunning:
			tr, rate := s.feed(cli, job)
			lane.Mode = "run"
			lane.Pct = tr.Percent(false)
			lane.Verb = tr.Verb
			lane.File = tr.File
			lane.Compiled = tr.Compiled()
			lane.Spark = rate.Spark()
			lane.PerSec = rate.PerSec()
			if eta, ok := rate.Last(); ok {
				lane.ETA = eta
				lane.ETAKnown = true
			}
		case queue.StatusQueued:
			place++
			lane.Mode = "wait"
			if lane.Position == 0 {
				lane.Position = place
			}
			lane.Detail = waitDetail(job, host, lane.Position)
		case queue.StatusSucceeded, queue.StatusFailed:
			lane.Mode = "done"
		default:
			continue
		}
		lanes = append(lanes, lane)
	}
	lanes = trimFinished(lanes, 3)
	m := ui.Machine{CPUs: host.CPUs, Busy: host.Busy, Running: host.Running, Queued: host.Queued}
	if host.CPUs == 0 {
		m.Running = runningN
		m.Queued = queuedN
	}
	return ui.Board(m, lanes), jobs, nil
}

func (s *session) feed(cli *client.Client, job *queue.Job) (*progress.Tracker, *progress.Rate) {
	id := job.ID
	tr := s.track[id]
	if tr == nil {
		tr = progress.New()
		s.track[id] = tr
	}
	rate := s.rate[id]
	if rate == nil {
		rate = progress.NewRate()
		s.rate[id] = rate
	}
	if !rate.HasPrior() && s.book != nil {
		if run, ok := s.book.Latest(job.Name, job.ID); ok {
			rate.UsePrior(progress.Prior{Files: run.Files, Duration: run.Duration})
		}
	}
	if logText, err := cli.Log(id); err == nil {
		if strings.HasPrefix(logText, s.have[id]) {
			tr.Feed(logText[len(s.have[id]):])
		} else {
			tr = progress.New()
			s.track[id] = tr
			tr.Feed(logText)
		}
		s.have[id] = logText
	}
	rate.Note(time.Now(), tr.Compiled(), tr.Percent(false), elapsed(job))
	return tr, rate
}

func (s *session) syncHistory(cli *client.Client, jobs []queue.Job) {
	if s.book == nil {
		book, err := history.Load()
		if err != nil {
			book = &history.Book{}
		}
		s.book = book
	}
	if s.noted == nil {
		s.noted = map[string]bool{}
	}
	best := map[string]queue.Job{}
	for i := range jobs {
		job := &jobs[i]
		if job.Status != queue.StatusSucceeded || job.Name == "" {
			continue
		}
		cur, ok := best[job.Name]
		if !ok || job.Finished.After(cur.Finished) {
			best[job.Name] = *job
		}
	}
	changed := false
	for _, job := range best {
		if s.book.Has(job.ID) || s.noted[job.ID] {
			continue
		}
		files, fetched := s.compileCount(cli, job.ID)
		if !fetched {
			continue
		}
		s.noted[job.ID] = true
		d := job.Finished.Sub(job.Started)
		if files < 1 || d < time.Second {
			continue
		}
		s.book.Put(history.Run{
			ID:       job.ID,
			Name:     job.Name,
			Files:    files,
			Duration: d,
			Ended:    job.Finished,
		})
		changed = true
	}
	if changed {
		_ = s.book.Save()
	}
}

func (s *session) compileCount(cli *client.Client, id string) (int, bool) {
	text, ok := s.have[id]
	if !ok {
		got, err := cli.Log(id)
		if err != nil {
			return 0, false
		}
		text = got
		s.have[id] = text
	}
	tr := progress.New()
	tr.Feed(text)
	return tr.Compiled(), true
}

func waitDetail(job *queue.Job, host queue.HostInfo, pos int) string {
	if host.CPUs > 0 && host.Busy+job.Jobs > host.CPUs {
		free := host.CPUs - host.Busy
		if free < 0 {
			free = 0
		}
		return fmt.Sprintf("needs %d cores, %d free", job.Jobs, free)
	}
	if pos > 1 {
		return "in line"
	}
	return "next"
}

func cpuHint() int {
	cfg, err := load()
	if err != nil || cfg.Jobs < 1 {
		return 0
	}
	return cfg.Jobs
}

func trimFinished(lanes []ui.Lane, keep int) []ui.Lane {
	var live, done []ui.Lane
	for _, lane := range lanes {
		if lane.Mode == "done" {
			done = append(done, lane)
			continue
		}
		live = append(live, lane)
	}
	if len(done) > keep {
		done = done[len(done)-keep:]
	}
	return append(live, done...)
}

func findJob(jobs []queue.Job, id string) *queue.Job {
	for i := range jobs {
		if jobs[i].ID == id {
			return &jobs[i]
		}
	}
	return nil
}

func cmdStatus(args []string) error {
	once := false
	for _, a := range args {
		if a == "--once" {
			once = true
			continue
		}
		return fmt.Errorf("unknown flag %q", a)
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	cli := clientFrom(cfg)
	jobs, err := cli.List()
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		ui.Info("queue empty")
		return nil
	}
	s := newSession(cfg.Jobs)
	if once || activeID(jobs) == "" {
		block, _, err := s.render(cli)
		if err != nil {
			return err
		}
		fmt.Print(block)
		return nil
	}
	var painted int
	var last string
	for {
		block, jobs, err := s.render(cli)
		if err != nil {
			return err
		}
		if ui.Enabled {
			painted = ui.Paint(os.Stdout, painted, block)
		} else if block != last {
			fmt.Print(block)
			last = block
		}
		if activeID(jobs) == "" {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func hintQueued(id string) {
	ui.Success("queued %s", id)
	fmt.Fprintf(ui.Out, "  %s  co status\n", ui.Dim("watch"))
	fmt.Fprintf(ui.Out, "  %s  co finish\n", ui.Dim("install"))
}

func displayName(job *queue.Job) string {
	if job.Name != "" {
		return job.Name
	}
	return "job " + job.ID
}

func elapsed(job *queue.Job) time.Duration {
	start := job.Started
	if start.IsZero() {
		start = job.Created
	}
	if start.IsZero() {
		return 0
	}
	end := time.Now()
	if !job.Finished.IsZero() && (job.Status == queue.StatusSucceeded || job.Status == queue.StatusFailed) {
		end = job.Finished
	}
	return end.Sub(start)
}

func activeID(jobs []queue.Job) string {
	var queued string
	for i := range jobs {
		switch jobs[i].Status {
		case queue.StatusRunning:
			return jobs[i].ID
		case queue.StatusQueued:
			queued = jobs[i].ID
		}
	}
	return queued
}

func sortJobs(jobs []queue.Job) {
	for i := 1; i < len(jobs); i++ {
		j := i
		for j > 0 && jobLess(jobs[j], jobs[j-1]) {
			jobs[j], jobs[j-1] = jobs[j-1], jobs[j]
			j--
		}
	}
}

func jobLess(a, b queue.Job) bool {
	ai, aerr := strconv.Atoi(a.ID)
	bi, berr := strconv.Atoi(b.ID)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a.ID < b.ID
}

func newestJob(jobs []queue.Job, name string) *queue.Job {
	sortJobs(jobs)
	var match *queue.Job
	for i := range jobs {
		if name != "" && jobs[i].Name == name {
			match = &jobs[i]
		}
	}
	if match != nil {
		return match
	}
	if len(jobs) == 0 {
		return nil
	}
	return &jobs[len(jobs)-1]
}

func logTail(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
