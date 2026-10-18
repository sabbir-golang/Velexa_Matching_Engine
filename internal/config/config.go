// Package config reads the engine's settings from the environment and decides where its data files live.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvOr returns the environment variable k, or d when it is unset or blank.
func EnvOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

// serverStateDir is where a deployed engine keeps its data files.
const serverStateDir = "/opt/velexa-sim-me"

// OnServer reports whether this is a deployed engine (serverStateDir exists).
func OnServer() bool {
	st, err := os.Stat(serverStateDir)
	return err == nil && st.IsDir()
}

// StateDir is where the engine keeps its data files (accounts, broker
// credentials, allowlist, audit trail, message log…): ME_STATE_DIR if set,
// else /opt/velexa-sim-me on a deployed server, else ./data. It is created if
// missing.
func StateDir() string {
	dir := "data"
	if v := EnvOr("ME_STATE_DIR", ""); v != "" {
		dir = strings.TrimRight(v, "/")
	} else if OnServer() {
		return serverStateDir
	}
	_ = os.MkdirAll(dir, 0o750)
	return dir
}

// StateFile is the default location of one of the engine's data files.
func StateFile(name string) string {
	return filepath.Join(StateDir(), name)
}

// SpecDir is where the FIX data dictionaries (FIXT11.xml, FIX50SP2.xml) live:
// the server's spec directory, else the spec/ folder shipped in the repository.
// ME_FIX_SPEC_DIR overrides both. Without a dictionary the FIX acceptor cannot start.
func SpecDir() string {
	if v := os.Getenv("ME_FIX_SPEC_DIR"); v != "" {
		return v
	}
	if OnServer() {
		return serverStateDir + "/spec"
	}
	return "spec"
}

// CheckStateDir makes sure the data folder can be written: broker credentials,
// the allowlist and order-book IDs are saved there, and a folder the engine
// cannot write would lose them at the next restart.
func CheckStateDir() error {
	f, err := os.CreateTemp(StateDir(), ".write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
