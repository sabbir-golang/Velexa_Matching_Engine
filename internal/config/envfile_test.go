package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "me.env")
	os.WriteFile(p, []byte("# comment\n\nPORT=8080\nexport FIX_PORT=5001\nNAME=\"quoted # not a comment\"\nSINGLE='a b'\nTRAILING=value # note\nEMPTY=\nbad line\n"), 0o600)
	vars, err := ReadEnvFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range vars {
		got[kv[0]] = kv[1]
	}
	want := map[string]string{"PORT": "8080", "FIX_PORT": "5001", "NAME": "quoted # not a comment", "SINGLE": "a b", "TRAILING": "value", "EMPTY": ""}
	if len(got) != len(want) {
		t.Fatalf("parsed %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// The environment wins over the file.
func TestEnvFileDoesNotOverrideTheEnvironment(t *testing.T) {
	p := filepath.Join(t.TempDir(), "me.env")
	os.WriteFile(p, []byte("ME_TEST_A=from-file\nME_TEST_B=from-file\n"), 0o600)
	t.Setenv("ME_ENV_FILE", p)
	t.Setenv("ME_TEST_A", "from-env")
	os.Unsetenv("ME_TEST_B")
	defer os.Unsetenv("ME_TEST_B")
	loadEnvFile()
	if os.Getenv("ME_TEST_A") != "from-env" || os.Getenv("ME_TEST_B") != "from-file" {
		t.Fatalf("A=%q B=%q", os.Getenv("ME_TEST_A"), os.Getenv("ME_TEST_B"))
	}
	if EnvFile != p {
		t.Fatalf("EnvFile = %q", EnvFile)
	}
}
