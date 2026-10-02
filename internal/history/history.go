// Package history remembers how long the last successful build of a project took.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const keepRuns = 32

// Run is one finished build.
type Run struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Files    int           `json:"files"`
	Duration time.Duration `json:"duration"`
	Ended    time.Time     `json:"ended"`
}

// Book is the on-disk list of finished builds.
type Book struct {
	path string
	Runs []Run `json:"runs"`
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "co", "history.json"), nil
}

// Load reads the history file. A missing file is an empty book.
func Load() (*Book, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return loadPath(path)
}

func loadPath(path string) (*Book, error) {
	b := &Book{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return b, nil
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, err
	}
	b.path = path
	return b, nil
}

func (b *Book) Has(id string) bool {
	for _, run := range b.Runs {
		if run.ID == id {
			return true
		}
	}
	return false
}

// Put records a finished build. A second put of the same id replaces it.
func (b *Book) Put(run Run) {
	if run.ID == "" || run.Name == "" || run.Files <= 0 || run.Duration <= 0 {
		return
	}
	for i := range b.Runs {
		if b.Runs[i].ID == run.ID {
			b.Runs[i] = run
			return
		}
	}
	b.Runs = append(b.Runs, run)
	if len(b.Runs) > keepRuns {
		b.Runs = b.Runs[len(b.Runs)-keepRuns:]
	}
}

// Latest is the newest finished build of name, other than exceptID.
func (b *Book) Latest(name, exceptID string) (Run, bool) {
	var best Run
	var found bool
	for _, run := range b.Runs {
		if run.Name != name || run.ID == exceptID {
			continue
		}
		if !found || run.Ended.After(best.Ended) {
			best = run
			found = true
		}
	}
	return best, found
}

func (b *Book) Save() error {
	if b.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(b.path, data, 0o644)
}
