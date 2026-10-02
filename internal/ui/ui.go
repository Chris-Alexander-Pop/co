// Package ui is the co terminal layout: a short banner, command list, and status lines.
// Styling turns off when stdout is not a terminal or NO_COLOR is set.
package ui

import (
	"fmt"
	"io"
	"os"
)

var (
	Out io.Writer = os.Stdout
	Err io.Writer = os.Stderr
)

var Enabled = detect()

func detect() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	brand  = "\033[38;5;39m"
	green  = "\033[38;5;78m"
	red    = "\033[38;5;203m"
	yellow = "\033[38;5;221m"
	gray   = "\033[38;5;244m"
	cyan   = "\033[38;5;80m"
)

func wrap(code, s string) string {
	if !Enabled {
		return s
	}
	return code + s + reset
}

func Bold(s string) string  { return wrap(bold, s) }
func Dim(s string) string   { return wrap(dim, s) }
func Brand(s string) string { return wrap(brand, s) }
func Green(s string) string { return wrap(green, s) }
func Red(s string) string   { return wrap(red, s) }
func Cyan(s string) string  { return wrap(cyan, s) }
func Gray(s string) string  { return wrap(gray, s) }

func Logo(tagline string) {
	if !Enabled {
		fmt.Fprintln(Out, "co — "+tagline)
		return
	}
	bar := brand + "▌" + reset
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %s %s%sco%s %s\n", bar, bold, brand, reset, Dim("compile offload"))
	fmt.Fprintf(Out, "  %s %s\n", bar, Dim(tagline))
}

func Heading(title string) {
	fmt.Fprintln(Out)
	if !Enabled {
		fmt.Fprintf(Out, "  == %s ==\n", title)
		return
	}
	fmt.Fprintf(Out, "  %s▌%s %s%s%s\n", brand, reset, bold, title, reset)
}

func Step(format string, a ...any) {
	mark := "->"
	if Enabled {
		mark = brand + "→" + reset
	}
	fmt.Fprintf(Out, "  %s %s\n", mark, fmt.Sprintf(format, a...))
}

func Success(format string, a ...any) {
	mark := "OK"
	if Enabled {
		mark = green + "✔" + reset
	}
	fmt.Fprintf(Out, "  %s %s\n", mark, fmt.Sprintf(format, a...))
}

func Fail(format string, a ...any) {
	mark := "X"
	if Enabled {
		mark = red + "✗" + reset
	}
	fmt.Fprintf(Err, "  %s %s\n", mark, fmt.Sprintf(format, a...))
}

func Info(format string, a ...any) {
	fmt.Fprintf(Out, "  %s %s\n", Gray("*"), Dim(fmt.Sprintf(format, a...)))
}

func ErrorBlock(msg string) {
	Fail("%s", msg)
}
