package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/aur"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func cmdAur(args []string) error {
	if len(args) == 0 || args[0] != "upgrade" {
		return fmt.Errorf("usage: co aur upgrade [--fallback local|cancel]")
	}
	modeFlag, rest, err := flagFallback(args[1:])
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("unknown flag %q", rest[0])
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	_, cli, err := reach(cfg, modeFlag)
	if err != nil {
		return err
	}
	if cli == nil {
		ui.Step("yay -Sua")
		return runLocal("yay", "-Sua", "--noconfirm")
	}
	out, err := exec.Command("yay", "-Qua").Output()
	if err != nil && len(out) == 0 {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) == 0 {
			ui.Success("no AUR upgrades")
			return nil
		}
		return fmt.Errorf("yay -Qua: %w", err)
	}
	upgrades := aur.ParseUpgrades(string(out))
	if len(upgrades) == 0 {
		ui.Success("no AUR upgrades")
		return nil
	}
	names := make([]string, len(upgrades))
	versions := map[string]string{}
	for i, u := range upgrades {
		names[i] = u.Name
		versions[u.Name] = u.New
	}
	info, err := aur.Info(names)
	if err != nil {
		return err
	}
	names, info, err = aur.WithDeps(names, info, aur.Info, repoSatisfies)
	if err != nil {
		return err
	}
	order := aur.Bases(names, info)
	ui.Step("queue %s", fmt.Sprint(order))
	dir, err := os.MkdirTemp("", "co-aur-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()
	tried := map[string]bool{}
	var stopped error
	for _, base := range order {
		if tried[base] {
			continue
		}
		if err := buildInstall(cli, base, versions[base], dir, tried); err != nil {
			if askSkipFailed(base, err) {
				continue
			}
			cleanup = false
			ui.Info("packages left in %s", dir)
			stopped = err
			break
		}
	}
	return stopped
}

// buildInstall pulls the package base, installs it here, then returns.
// The next base is not submitted until this install finishes.
// A missing AUR dependency is built and installed, and the install is tried once more.
func buildInstall(cli *client.Client, base, version, dir string, tried map[string]bool) error {
	if tried[base] {
		return fmt.Errorf("%s already attempted", base)
	}
	tried[base] = true
	paths, err := oneAur(cli, base, version, filepath.Join(dir, base))
	if len(paths) == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s produced no packages", base)
	}
	return installWithAURDeps(cli, base, paths, dir, tried)
}

func installWithAURDeps(cli *client.Client, base string, paths []string, dir string, tried map[string]bool) error {
	ui.Step("install %s", base)
	err := installPkgs(paths)
	if err == nil {
		return nil
	}
	missing := aur.MissingDeps(commandOutput(err))
	var built int
	for _, dep := range missing {
		info, ierr := aur.Info([]string{dep})
		if ierr != nil {
			return err
		}
		pkg, ok := info[dep]
		if !ok || pkg.Base == "" || pkg.Base == base || tried[pkg.Base] {
			continue
		}
		ui.Step("%s needs %s from the AUR", base, pkg.Base)
		if derr := buildInstall(cli, pkg.Base, "", dir, tried); derr != nil {
			return err
		}
		built++
	}
	if built == 0 {
		return err
	}
	ui.Step("install %s", base)
	return installPkgs(paths)
}

func oneAur(cli *client.Client, name, version, dest string) ([]string, error) {
	if paths, ok, err := reuseAur(cli, name, version, dest); ok || err != nil {
		return paths, err
	}
	ui.Step("build %s", name)
	job, err := cli.SubmitAur(name)
	if err != nil {
		return nil, err
	}
	noteSame(job)
	_, waitErr := waitJob(cli, job.ID)
	paths, dlErr := downloadPkgs(cli, job.ID, dest)
	if len(paths) == 0 {
		if waitErr != nil {
			return nil, waitErr
		}
		if dlErr != nil {
			return nil, dlErr
		}
		return nil, nil
	}
	if waitErr != nil {
		ui.Info("builder reported %s: %v", name, waitErr)
		ui.Info("pulled %d package file(s) anyway", len(paths))
	}
	return paths, nil
}

// reuseAur downloads a finished build of this version instead of compiling again.
// A finished AUR job is not reused by the builder, because a later yay -Qua
// must be allowed to build a newer release. The package file name carries the
// version, so a match is the same build the client failed to install.
func reuseAur(cli *client.Client, name, version, dest string) ([]string, bool, error) {
	if version == "" {
		return nil, false, nil
	}
	jobs, err := cli.List()
	if err != nil {
		return nil, false, err
	}
	var newest []queue.Job
	for _, job := range jobs {
		if job.Kind != queue.KindAur || job.Name != name || job.Status != queue.StatusSucceeded {
			continue
		}
		newest = append(newest, job)
	}
	for i := 0; i < len(newest); i++ {
		for j := i + 1; j < len(newest); j++ {
			if newerJob(newest[j], newest[i]) {
				newest[i], newest[j] = newest[j], newest[i]
			}
		}
	}
	for _, job := range newest {
		files, err := cli.Artifacts(job.ID)
		if err != nil || !anyArtifact(name, version, files) {
			continue
		}
		ui.Step("already built %s %s", name, version)
		paths, err := downloadPkgs(cli, job.ID, dest)
		return paths, true, err
	}
	return nil, false, nil
}

func newerJob(a, b queue.Job) bool {
	if a.Finished.Equal(b.Finished) {
		return a.Created.After(b.Created)
	}
	return a.Finished.After(b.Finished)
}

func anyArtifact(name, version string, files []string) bool {
	for _, file := range files {
		if aur.ArtifactHasVersion(name, version, file) {
			return true
		}
	}
	return false
}

func askSkipFailed(name string, buildErr error) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer tty.Close()
	fmt.Fprintf(tty, "%s failed: %v. Skip %s and continue? [y/N] ", name, buildErr, name)
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	return skipAndContinue(line)
}

func skipAndContinue(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// repoSatisfies reports whether installed packages or the sync databases
// already meet spec. pacman -T covers a provide such as sdl2-compat
// providing sdl2. pacman -Sp covers a repo package that is not installed yet.
func repoSatisfies(spec string) bool {
	if strings.TrimSpace(spec) == "" {
		return false
	}
	if pacmanQuiet("-T", spec) {
		return true
	}
	return pacmanQuiet("-Sp", "--print-format", "%n", spec)
}

func pacmanQuiet(args ...string) bool {
	cmd := exec.Command("pacman", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}
