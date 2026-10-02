// Package fallback decides what to do when the compile queue cannot be reached.
package fallback

import "fmt"

const (
	Local  = "local"
	Cancel = "cancel"
)

// Choose returns the mode to use. A flag overrides config. An empty flag uses config.
func Choose(flag, configured string) (string, error) {
	mode := flag
	if mode == "" {
		mode = configured
	}
	if mode == "" {
		mode = Local
	}
	if mode != Local && mode != Cancel {
		return "", fmt.Errorf("fallback must be local or cancel, got %q", mode)
	}
	return mode, nil
}
