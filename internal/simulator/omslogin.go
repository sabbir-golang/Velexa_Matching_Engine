package simulator

// omslogin.go — the market bots' saved OMS login. With Remember ticked, a
// successful sign-in is saved to sim_oms.json in the data folder (mode 0600,
// like the broker credential files), so the next Start can leave the password
// blank. The password never leaves the engine: the page and the audit trail
// see only the address, the login and when it was saved.

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/engine"
)

type savedLogin struct {
	URL      string    `json:"url"`
	API      string    `json:"api,omitempty"`
	User     string    `json:"user"`
	Password string    `json:"password"`
	Saved    time.Time `json:"saved"`
}

var loginMu sync.Mutex

func loginFile() string { return config.StateFile("sim_oms.json") }

// savedOMSLogin reads the saved login; ok is false when there is none.
func savedOMSLogin() (l savedLogin, ok bool) {
	loginMu.Lock()
	defer loginMu.Unlock()
	b, err := os.ReadFile(loginFile())
	if err != nil || json.Unmarshal(b, &l) != nil || l.User == "" || l.Password == "" {
		return savedLogin{}, false
	}
	return l, true
}

// saveOMSLogin replaces the saved login through a temporary file, so a crash
// never leaves half a credential file.
func saveOMSLogin(l savedLogin) error {
	l.Saved = time.Now()
	b, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		return err
	}
	loginMu.Lock()
	defer loginMu.Unlock()
	tmp := loginFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, loginFile())
}

// ForgetOMSLogin deletes the saved login.
func (s *Sim) ForgetOMSLogin() error {
	loginMu.Lock()
	defer loginMu.Unlock()
	if err := os.Remove(loginFile()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// savedPassword is the saved password for this login on this OMS, or "".
func savedPassword(site omsSite, user string) string {
	l, ok := savedOMSLogin()
	if !ok || !strings.EqualFold(strings.TrimSpace(l.User), strings.TrimSpace(user)) {
		return ""
	}
	if ls, err := parseOMSSite(l.URL, l.API); err != nil || ls.Origin != site.Origin || ls.API != site.API {
		return ""
	}
	return l.Password
}

// savedLoginStatus is what the page may see of the saved login: never the password.
func savedLoginStatus() map[string]any {
	l, ok := savedOMSLogin()
	if !ok {
		return nil
	}
	return map[string]any{"url": l.URL, "user": l.User, "saved": l.Saved.In(engine.Dhaka).Format("2 Jan 15:04")}
}
