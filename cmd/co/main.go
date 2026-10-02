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
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		ui.ErrorBlock(err.Error())
		os.Exit(1)
	}
}

func usage() {
	ui.Logo("queue compiles on a builder, or build here if it is down")

	type cmd struct{ name, args, desc string }
	cmds := []cmd{
		{"ship", "[--no-wait] [--fallback local|cancel]", "queue this directory (makepkg or make)"},
		{"makepkg", "[--no-install] [--fallback local|cancel]", "build this PKGBUILD on the queue and install it"},
		{"aur", "upgrade [--fallback local|cancel]", "upgrade AUR packages via the queue"},
		{"status", "", "list jobs on the builder"},
		{"logs", "[id]", "print a job log (default: newest)"},
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
		return cmdStatus()
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
