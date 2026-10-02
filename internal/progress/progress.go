// Package progress turns a kernel or package build log into one moving status.
package progress

import (
	"regexp"
	"strings"
)

// Tracker reads build output and keeps the last file plus a percentage.
// The percentage climbs while files compile, then sits under 100 until the job ends.
type Tracker struct {
	partial  string
	compiled int
	installs int
	target   int
	phase    string
	Verb     string
	File     string
}

func New() *Tracker {
	return &Tracker{target: 200, phase: "compile"}
}

var action = regexp.MustCompile(`^[ \t]*(CC(?: \[[A-Z]\])?|AS|LD|AR|LDS|X32|VDSO2C|VDSO|OBJCOPY|GEN|HOSTCC|HOSTLD|CHK|SHIPPED|INSTALL|DEPMOD|MODPOST|WRAP|CPP)[ \t]+(\S+)`)

// Feed adds the next chunk of log text. Chunks may split a line.
func (t *Tracker) Feed(chunk string) {
	if chunk == "" {
		return
	}
	t.partial += chunk
	for {
		i := strings.IndexByte(t.partial, '\n')
		if i < 0 {
			return
		}
		line := strings.TrimRight(t.partial[:i], "\r")
		t.partial = t.partial[i+1:]
		t.note(line)
	}
}

func (t *Tracker) note(line string) {
	m := action.FindStringSubmatch(line)
	if m == nil {
		return
	}
	verb, file := m[1], m[2]
	t.Verb = verb
	t.File = file
	switch {
	case verb == "DEPMOD":
		t.phase = "depmod"
	case verb == "INSTALL" && strings.Contains(file, "lib/modules"):
		t.phase = "install"
		t.installs++
	default:
		if t.phase == "compile" {
			t.compiled++
		}
	}
}

// Compiled is how many compile lines have been seen.
func (t *Tracker) Compiled() int { return t.compiled }

// Percent is 100 only when done is true.
func (t *Tracker) Percent(done bool) int {
	if done {
		return 100
	}
	switch t.phase {
	case "depmod":
		return 98
	case "install":
		p := 88 + t.installs/30
		if p > 97 {
			return 97
		}
		return p
	default:
		expect := t.target
		if t.compiled > 400 {
			expect = 12000
		}
		if t.compiled+400 > expect {
			expect = t.compiled + 400
		}
		if expect < 1 {
			return 0
		}
		p := t.compiled * 85 / expect
		if p > 85 {
			return 85
		}
		if p < 1 && t.compiled > 0 {
			return 1
		}
		return p
	}
}
