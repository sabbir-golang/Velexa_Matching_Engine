package brokers

import (
	"os"
	"path/filepath"
	"testing"
)

// A credential change that cannot be written must say so: otherwise the broker
// would be told it is registered and lose it at the next restart.
func TestSaveFailuresAreReported(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ro")
	os.Mkdir(dir, 0o500)
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	fix := &FixAuth{Mode: "auto", path: filepath.Join(dir, "fix.json"), creds: map[string]string{}}
	if err := fix.Add("AAA", "secret"); err == nil {
		t.Fatal("Fix.Add into a read-only folder returned no error")
	}
	m := &brokerMeta{path: filepath.Join(dir, "meta.json"), recs: map[string]*Record{}}
	if err := m.Add(&Record{Sender: "AAA"}); err == nil {
		t.Fatal("Meta.Add into a read-only folder returned no error")
	}
	ok := filepath.Join(t.TempDir(), "fix.json")
	fix.path = ok
	if err := fix.Add("BBB", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ok + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
}
