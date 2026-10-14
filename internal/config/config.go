// Package config reads the engine's settings from the environment and decides where its data files live.
package config

import (
	"os"
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

// StateFile is the default location of one of the engine's data files (broker
// credentials, allowlist, audit trail, message log…). A deployed engine keeps
// them under /opt/velexa-sim-me; on a machine without that directory (a
// developer's laptop) they go in ./data instead, so saving a broker's
// credentials works instead of failing with "no such directory".
func StateFile(name string) string {
	if OnServer() {
		return serverStateDir + "/" + name
	}
	_ = os.MkdirAll("data", 0o750)
	return "data/" + name
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
