package config

// The engine takes its settings from environment variables. So that `go run .`
// or `./bin/velexa-sim-me` picks up a local env file without exporting it first,
// the file is loaded when the program starts — before any other package reads
// a setting, because every package that reads one imports config.
//
// File: ME_ENV_FILE if set, otherwise ./me.env, otherwise (on a server)
// /opt/velexa-sim-me/me.env. A variable that is already set in the environment
// is left alone: the environment wins over the file.

import (
	"bufio"
	"log"
	"os"
	"strings"
)

func init() { loadEnvFile() }

// EnvFile is the env file that was loaded at start-up ("" when none).
var EnvFile string

func envFilePath() string {
	if p := strings.TrimSpace(os.Getenv("ME_ENV_FILE")); p != "" {
		return p
	}
	if _, err := os.Stat("me.env"); err == nil {
		return "me.env"
	}
	if OnServer() {
		if p := serverStateDir + "/me.env"; fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func loadEnvFile() {
	path := envFilePath()
	if path == "" {
		return
	}
	vars, err := ReadEnvFile(path)
	if err != nil {
		log.Printf("[config] could not read %s: %v", path, err)
		return
	}
	set, kept := 0, 0
	for _, kv := range vars {
		if _, exists := os.LookupEnv(kv[0]); exists {
			kept++
			continue
		}
		os.Setenv(kv[0], kv[1])
		set++
	}
	EnvFile = path
	log.Printf("[config] loaded %d settings from %s (%d already set in the environment were kept)", set, path, kept)
}

// ReadEnvFile parses a KEY=VALUE file: blank lines and lines starting with #
// are skipped, an "export " prefix is allowed, a value may be wrapped in single
// or double quotes, and an unquoted value ends at " #" (a trailing comment).
// Values are not expanded.
func ReadEnvFile(path string) ([][2]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][2]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		val = strings.TrimSpace(val)
		if n := len(val); n >= 2 && (val[0] == '"' || val[0] == '\'') && val[n-1] == val[0] {
			val = val[1 : n-1]
		} else if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		out = append(out, [2]string{key, val})
	}
	return out, sc.Err()
}
