package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/config"
	"github.com/Chris-Alexander-Pop/co/internal/fallback"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func reach(cfg *config.Config, flag string) (mode string, cli *client.Client, err error) {
	mode, err = fallback.Choose(flag, cfg.Fallback)
	if err != nil {
		return "", nil, err
	}
	cli = client.New(cfg.URL, cfg.Token)
	if err := cli.Healthy(); err != nil {
		ui.Info("builder unavailable: %s", err)
		if mode == fallback.Cancel {
			return mode, nil, fmt.Errorf("builder unavailable, fallback is cancel")
		}
		ui.Info("building on this machine")
		return mode, nil, nil
	}
	return mode, cli, nil
}

func packCwd() ([]byte, error) {
	var buf bytes.Buffer
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := archive.Pack(wd, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func waitJob(cli *client.Client, id string) (*queue.Job, error) {
	job, err := follow(cli, id)
	if err != nil {
		return job, err
	}
	return job, nil
}

func downloadPkgs(cli *client.Client, id, dest string) ([]string, error) {
	names, err := cli.Artifacts(id)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, name := range names {
		path := filepath.Join(dest, name)
		ui.Step("pull %s", name)
		if err := cli.Download(id, name, path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func installPkgs(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no packages were built")
	}
	args := append([]string{"pacman", "-U", "--noconfirm"}, paths...)
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runLocal(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
