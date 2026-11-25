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

var (
	action    = regexp.MustCompile(`^[ \t]*(CC(?: \[[A-Z]\])?|AS|LD|AR|LDS|X32|VDSO2C|VDSO|OBJCOPY|GEN|HOSTCC|HOSTLD|CHK|SHIPPED|INSTALL|DEPMOD|MODPOST|WRAP|CPP)[ \t]+(\S+)`)
	phaseLine = regexp.MustCompile(`^==> Starting ([A-Za-z]+)\(\)`)
	testFile  = regexp.MustCompile(`^Running\s+(\S+\.exp)\b`)
	srcFile   = regexp.MustCompile(`(?:^|\s)(\S+\.(?:cc|cpp|cxx|f90|go|c|C|s|S|f|F))(?:\s|\\)*$`)
	objFile   = regexp.MustCompile(`(?:^|\s)-o\s+(\S+\.o)\b`)
)

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
	if m := action.FindStringSubmatch(line); m != nil {
		t.see(m[1], m[2], true)
		return
	}
	if m := phaseLine.FindStringSubmatch(line); m != nil {
		switch m[1] {
		case "check":
			t.phase = "check"
		case "package":
			t.phase = "package"
		case "build":
			t.phase = "compile"
		}
		t.Verb = m[1]
		t.File = ""
		return
	}
	if m := testFile.FindStringSubmatch(line); m != nil {
		t.phase = "check"
		t.Verb = "check"
		t.File = baseName(m[1])
		return
	}
	if file := compileName(line); file != "" {
		t.see("CC", file, t.phase == "compile")
	}
}

func (t *Tracker) see(verb, file string, count bool) {
	t.Verb = verb
	t.File = file
	switch {
	case verb == "DEPMOD":
		t.phase = "depmod"
	case verb == "INSTALL" && strings.Contains(file, "lib/modules"):
		t.phase = "install"
		t.installs++
	default:
		if count && t.phase == "compile" {
			t.compiled++
		}
	}
}

func compileName(line string) string {
	if m := srcFile.FindStringSubmatch(line); m != nil {
		return baseName(m[1])
	}
	if m := objFile.FindStringSubmatch(line); m != nil {
		return baseName(m[1])
	}
	return ""
}

func baseName(p string) string {
	p = strings.Trim(p, `"'`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
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
	case "package":
		return 96
	case "check":
		return 90
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
