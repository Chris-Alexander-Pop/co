package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Chris-Alexander-Pop/co/internal/aur"
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
	names := aur.ParseQua(string(out))
	if len(names) == 0 {
		ui.Success("no AUR upgrades")
		return nil
	}
	deps, err := aur.Deps(names)
	if err != nil {
		return err
	}
	order := aur.Order(names, deps)
	ui.Step("queue %s", fmt.Sprint(order))
	dir, err := os.MkdirTemp("", "co-aur-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var built []string
	for _, name := range order {
		ui.Step("build %s", name)
		job, err := cli.SubmitAur(name)
		if err != nil {
			return err
		}
		if _, err := waitJob(cli, job.ID); err != nil {
			return err
		}
		paths, err := downloadPkgs(cli, job.ID, filepath.Join(dir, name))
		if err != nil {
			return err
		}
		built = append(built, paths...)
	}
	ui.Step("install %d package(s)", len(built))
	return installPkgs(built)
}
