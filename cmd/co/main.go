package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/config"
	"github.com/Chris-Alexander-Pop/co/internal/fallback"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func main() {
	args := os.Args[1:]
	var err error
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		err = cmdProject(args)
	} else {
		err = run(args[0], args[1:])
	}
	if err != nil {
		ui.ErrorBlock(err.Error())
		os.Exit(1)
	}
}

func usage() {
	ui.Logo("build on the queue, then install here")

	type cmd struct{ name, args, desc string }
	cmds := []cmd{
		{"", "[--bg] [--fallback local|cancel]", "build this directory from its strategy, or its PKGBUILD"},
		{"makepkg", "[--no-install] [--bg] [--fallback local|cancel]", "build this PKGBUILD on the queue and install it"},
		{"aur", "upgrade [--fallback local|cancel]", "upgrade AUR packages via the queue"},
		{"status", "[--once]", "live build meter, or one snapshot"},
		{"finish", "", "wait for this directory's job, then install it"},
		{"logs", "[id]", "print the raw build log"},
	}
	ui.Heading("Commands")
	w := 0
	for _, c := range cmds {
		if l := len(c.name + " " + c.args); l > w {
			w = l
		}
	}
	for _, c := range cmds {
		label := c.name
		if c.args != "" {
			label += " " + ui.Dim(c.args)
		}
		pad := strings.Repeat(" ", w-len(c.name+" "+c.args))
		fmt.Fprintf(ui.Out, "  %s %s%s  %s\n", ui.Brand("co"), ui.Bold(label), pad, ui.Dim(c.desc))
	}
	ui.Heading("Paths")
	fmt.Fprintf(ui.Out, "  config  %s\n", ui.Dim("~/.config/co/config.yaml"))
}

func run(cmd string, args []string) error {
	switch cmd {
	case "-h", "--help", "help":
		usage()
		return nil
	case "ship":
		return cmdShip(args)
	case "makepkg":
		return cmdMakepkg(args)
	case "aur":
		return cmdAur(args)
	case "status":
		return cmdStatus(args)
	case "finish":
		return cmdFinish(args)
	case "logs":
		return cmdLogs(args)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func flagFallback(args []string) (string, []string, error) {
	var mode string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--fallback" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--fallback needs local or cancel")
			}
			mode = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--fallback=") {
			mode = strings.TrimPrefix(a, "--fallback=")
			continue
		}
		rest = append(rest, a)
	}
	if mode != "" {
		if _, err := fallback.Choose(mode, ""); err != nil {
			return "", nil, err
		}
	}
	return mode, rest, nil
}

func load() (*config.Config, error) {
	return config.Load()
}
