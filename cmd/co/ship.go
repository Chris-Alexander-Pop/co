package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/config"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func cmdShip(args []string) error {
	modeFlag, rest, err := flagFallback(args)
	if err != nil {
		return err
	}
	noWait := false
	for _, a := range rest {
		if a == "--no-wait" {
			noWait = true
			continue
		}
		return fmt.Errorf("unknown flag %q", a)
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	kind := queue.KindMake
	if _, err := os.Stat(filepath.Join(wd, "PKGBUILD")); err == nil {
		kind = queue.KindMakepkg
	}
	_, cli, err := reach(cfg, modeFlag)
	if err != nil {
		return err
	}
	if cli == nil {
		if kind == queue.KindMakepkg {
			return runLocal("makepkg", "-f", "--noconfirm", "--syncdeps")
		}
		return runLocal("make", fmt.Sprintf("-j%d", cfg.Jobs))
	}
	ui.Step("packing %s", filepath.Base(wd))
	blob, err := packCwd()
	if err != nil {
		return err
	}
	job, err := cli.SubmitTree(kind, filepath.Base(wd), cfg.Jobs, blob)
	if err != nil {
		return err
	}
	ui.Success("queued %s (%s)", job.ID, kind)
	if noWait {
		return nil
	}
	if _, err := waitJob(cli, job.ID); err != nil {
		return err
	}
	if kind == queue.KindMakepkg {
		paths, err := downloadPkgs(cli, job.ID, wd)
		if err != nil {
			return err
		}
		return installPkgs(paths)
	}
	ui.Success("build finished")
	return nil
}

func cmdMakepkg(args []string) error {
	modeFlag, rest, err := flagFallback(args)
	if err != nil {
		return err
	}
	install := true
	for _, a := range rest {
		if a == "--no-install" {
			install = false
			continue
		}
		return fmt.Errorf("unknown flag %q", a)
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(wd, "PKGBUILD")); err != nil {
		return fmt.Errorf("no PKGBUILD in %s", wd)
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
		return runLocal("makepkg", "-f", "--noconfirm", "--syncdeps")
	}
	ui.Step("packing %s", filepath.Base(wd))
	blob, err := packCwd()
	if err != nil {
		return err
	}
	job, err := cli.SubmitTree(queue.KindMakepkg, filepath.Base(wd), cfg.Jobs, blob)
	if err != nil {
		return err
	}
	if _, err := waitJob(cli, job.ID); err != nil {
		return err
	}
	paths, err := downloadPkgs(cli, job.ID, wd)
	if err != nil {
		return err
	}
	if !install {
		ui.Success("packages downloaded")
		return nil
	}
	return installPkgs(paths)
}

func cmdStatus() error {
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
	for _, job := range jobs {
		fmt.Printf("%s  %s  %s  %s\n", job.ID, job.Status, job.Kind, job.Name)
	}
	return nil
}

func cmdLogs(args []string) error {
	cfg, err := load()
	if err != nil {
		return err
	}
	cli := clientFrom(cfg)
	id := ""
	if len(args) > 0 {
		id = args[0]
	} else {
		jobs, err := cli.List()
		if err != nil {
			return err
		}
		var newest string
		for _, job := range jobs {
			if job.ID > newest {
				newest = job.ID
				id = job.ID
			}
		}
		if id == "" {
			return fmt.Errorf("queue empty")
		}
	}
	text, err := cli.Log(id)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

func clientFrom(cfg *config.Config) *client.Client {
	return client.New(cfg.URL, cfg.Token)
}
