package config

import "os"

// Version is the engine's build version, set when the binary is built:
//
//	go build -ldflags "-X velexa-sim-me/internal/config.Version=1.4.0"
//
// The Docker image sets it from the git tag or commit.
var Version = "dev"

// InContainer reports whether the engine runs inside a container (Docker or
// Podman), where this machine's own IP is not the address brokers can reach.
func InContainer() bool {
	if os.Getenv("ME_IN_CONTAINER") == "1" {
		return true
	}
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}
