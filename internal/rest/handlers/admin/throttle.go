package admin

import (
	"sync"
	"time"
)

// ── login brute-force throttle ─────────────────────────────────────────────
type failRec struct {
	count int
	until time.Time
}

var (
	loginMu    sync.Mutex
	loginFails = map[string]*failRec{}
)

const maxLoginFails = 5

func loginBlocked(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[ip]
	return r != nil && r.count >= maxLoginFails && time.Now().Before(r.until)
}

func noteLoginFail(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[ip]
	if r == nil {
		r = &failRec{}
		loginFails[ip] = r
	}
	r.count++
	r.until = time.Now().Add(5 * time.Minute)
}

func ClearLoginFail(ip string) {
	loginMu.Lock()
	delete(loginFails, ip)
	loginMu.Unlock()
}
