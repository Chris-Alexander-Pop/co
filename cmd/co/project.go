package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/strategy"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func cmdProject(args []string) error {
	modeFlag, rest, err := flagFallback(args)
	if err != nil {
		return err
	}
	bg := false
	for _, a := range rest {
		if a == "--no-wait" || a == "--bg" {
			bg = true
			continue
		}
		return fmt.Errorf("unknown flag %q", a)
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	projects, err := strategy.ProjectsDir()
	if err != nil {
		return err
	}
	file, err := strategy.Find(projects, wd)
	if err != nil {
		return err
	}
	if file == nil {
		if _, statErr := os.Stat(filepath.Join(wd, "PKGBUILD")); statErr == nil {
			return cmdMakepkg(args)
		}
		return fmt.Errorf("no strategy for %s and no PKGBUILD", wd)
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	build := strategy.Expand(file.Build, cfg.Jobs, "")
	_, cli, err := reach(cfg, modeFlag)
	if err != nil {
		return err
	}
	if cli == nil {
		if err := runShell(wd, build); err != nil {
			return err
		}
		return runInstall(wd, file.Install, cfg.Jobs, kernelRelease(wd))
	}
	ui.Step("packing %s", filepath.Base(wd))
	var buf bytes.Buffer
	if err := archive.PackFilter(wd, &buf, file.SkipRel); err != nil {
		return err
	}
	job, err := cli.SubmitStrategy(filepath.Base(wd), cfg.Jobs, build, file.Pull, buf.Bytes())
	if err != nil {
		return err
	}
	if bg {
		hintQueued(job.ID)
		return nil
	}
	done, err := waitJob(cli, job.ID)
	if err != nil {
		return err
	}
	return deliver(cli, done, file, wd, cfg.Jobs)
}

func deliver(cli *client.Client, job *queue.Job, file *strategy.File, wd string, jobs int) error {
	if len(file.Pull) > 0 {
		tarPath := filepath.Join(wd, queue.PullArtifact)
		ui.Step("pull %s", queue.PullArtifact)
		if err := cli.Download(job.ID, queue.PullArtifact, tarPath); err != nil {
			return err
		}
		f, err := os.Open(tarPath)
		if err != nil {
			return err
		}
		unpackErr := archive.Unpack(f, wd)
		f.Close()
		os.Remove(tarPath)
		if unpackErr != nil {
			return unpackErr
		}
	}
	return runInstall(wd, file.Install, jobs, job.KernelRelease)
}

func cmdFinish(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown flag %q", args[0])
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	projects, err := strategy.ProjectsDir()
	if err != nil {
		return err
	}
	file, err := strategy.Find(projects, wd)
	if err != nil {
		return err
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
	job := newestJob(jobs, filepath.Base(wd))
	if job == nil {
		return fmt.Errorf("queue empty")
	}
	if job.Status == queue.StatusQueued || job.Status == queue.StatusRunning {
		done, err := waitJob(cli, job.ID)
		if err != nil {
			return err
		}
		job = done
	}
	if job.Status != queue.StatusSucceeded {
		return fmt.Errorf("job %s is %s", job.ID, job.Status)
	}
	if file != nil && job.Kind == queue.KindStrategy {
		return deliver(cli, job, file, wd, cfg.Jobs)
	}
	if job.Kind == queue.KindMakepkg {
		paths, err := downloadPkgs(cli, job.ID, wd)
		if err != nil {
			return err
		}
		return installPkgs(paths)
	}
	return fmt.Errorf("job %s has nothing to install", job.ID)
}

func runInstall(dir string, lines []string, jobs int, release string) error {
	install := strategy.Expand(lines, jobs, release)
	for _, line := range install {
		if strings.Contains(line, "{kernelrelease}") {
			return fmt.Errorf("kernelrelease is empty, so this command cannot run: %s", line)
		}
	}
	return runShell(dir, install)
}

func runShell(dir string, lines []string) error {
	for _, line := range lines {
		ui.Step("%s", line)
		cmd := exec.Command("bash", "-c", line)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", line, err)
		}
	}
	return nil
}

func kernelRelease(dir string) string {
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
