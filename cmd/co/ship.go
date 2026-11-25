package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/config"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func cmdShip(args []string) error {
	return fmt.Errorf("co ship is gone. Run co in the project directory")
}

func cmdMakepkg(args []string) error {
	modeFlag, rest, err := flagFallback(args)
	if err != nil {
		return err
	}
	seed, bg, rest, err := flagBuildOpts(rest)
	if err != nil {
		return err
	}
	install := true
	for _, a := range rest {
		switch a {
		case "--no-install":
			install = false
		default:
			return fmt.Errorf("unknown flag %q", a)
		}
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
	if seed {
		if cli == nil {
			return fmt.Errorf("builder unavailable, --seed-cache needs the queue")
		}
		dirs, env, err := prepareSeed()
		if err != nil {
			return err
		}
		ui.Info("building on this machine to seed the builder cache")
		if err := runLocal("makepkg", env, "-f", "--noconfirm", "--syncdeps"); err != nil {
			return err
		}
		if err := uploadSeed(cli, dirs); err != nil {
			return err
		}
		if err := pruneSeed(dirs); err != nil {
			return err
		}
		if !install {
			ui.Success("packages built locally")
			return nil
		}
		paths, err := localPkgFiles(wd)
		if err != nil {
			return err
		}
		return installPkgs(paths)
	}
	if cli == nil {
		return runLocal("makepkg", nil, "-f", "--noconfirm", "--syncdeps")
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
	noteSame(job)
	if bg {
		hintQueued(job.ID)
		return nil
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

func localPkgFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, ".pkg.tar.") && !strings.HasSuffix(name, ".sig") {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths, nil
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
