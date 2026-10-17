package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/users"
)

func TestTemplateMatchesExampleFile(t *testing.T) {
	b, err := os.ReadFile("../../me.env.example")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != config.EnvTemplate() {
		t.Fatal("me.env.example and envTemplate differ: keep them identical")
	}
}

func TestFirstRunEnvFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "me.env")
	if err := config.WriteFirstRunEnv(p); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", st.Mode().Perm())
	}
	vars, _ := config.ReadEnvFile(p)
	got := map[string]string{}
	for _, kv := range vars {
		got[kv[0]] = kv[1]
	}
	if err := users.CheckPassword(got["ME_ADMIN_USER"], got["ME_ADMIN_PASS"]); err != nil || got["ME_ADMIN_USER"] != "admin" {
		t.Fatalf("generated admin password rejected: %v", err)
	}
	if len(got["ME_ADMIN_SECRET"]) < 32 {
		t.Fatalf("session secret too short: %d", len(got["ME_ADMIN_SECRET"]))
	}
	for k := range got {
		if !strings.HasPrefix(k, "ME_ADMIN_") {
			t.Errorf("%s is set; only the admin settings should be active in a generated file", k)
		}
	}
	before, _ := os.ReadFile(p)
	if config.WriteFirstRunEnv(p) == nil {
		t.Fatal("an existing env file must never be overwritten")
	}
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Fatal("existing env file changed")
	}
}

func TestStateDirSetting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	t.Setenv("ME_STATE_DIR", dir)
	if got := config.StateFile("users.json"); got != filepath.Join(dir, "users.json") {
		t.Fatalf("StateFile = %q", got)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatal("state dir not created")
	}
}

func TestLocalIP(t *testing.T) {
	if ip := config.LocalIP(); ip == "" || strings.Contains(ip, ":") && !strings.Contains(ip, ".") && ip != "::1" {
		t.Fatalf("LocalIP = %q", ip)
	}
	if config.Hostname() == "" {
		t.Fatal("no hostname")
	}
}
