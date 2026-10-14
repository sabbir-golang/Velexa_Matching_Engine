package register

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/users"
	"velexa-sim-me/internal/rest/middleware"
)

const (
	maxRegistrations  = 5 // sign-up attempts per client IP per window
	registrationSpan  = time.Hour
	maxTrackedIPs     = 50_000
	registerErrPrefix = "reg_"
)

var (
	regMu   sync.Mutex
	regSeen = map[string][]time.Time{}
)

// registrationAllowed counts one sign-up attempt for the IP and reports whether
// it is within the hourly quota (only with ME_RATE_LIMIT=on). Every attempt counts, failed ones too, so the
// form cannot be used to probe for taken names or emails at speed.
func registrationAllowed(ip string) bool {
	if !config.RateLimit() {
		return true
	}
	regMu.Lock()
	defer regMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-registrationSpan)
	if len(regSeen) >= maxTrackedIPs {
		for k, ts := range regSeen {
			if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
				delete(regSeen, k)
			}
		}
		if len(regSeen) >= maxTrackedIPs {
			return false // under a flood: refuse rather than grow
		}
	}
	kept := regSeen[ip][:0]
	for _, t := range regSeen[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= maxRegistrations {
		regSeen[ip] = kept
		return false
	}
	regSeen[ip] = append(kept, now)
	return true
}

// Register serves POST /register: creates a "user" account, then returns to the sign-in form.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	fail := func(code string) {
		http.Redirect(w, r, "/?e="+registerErrPrefix+code+"&as=register", http.StatusSeeOther)
	}
	if !config.RegistrationOpen() || h.Users == nil {
		fail("off")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := r.ParseForm(); err != nil {
		fail("off")
		return
	}
	ip := middleware.ClientIP(r)
	if !registrationAllowed(ip) {
		fail("rate")
		return
	}
	name, pass := strings.TrimSpace(r.FormValue("user")), r.FormValue("pass")
	if pass != r.FormValue("pass2") {
		fail("mismatch")
		return
	}
	prof := users.Profile{
		FullName: r.FormValue("full_name"), Email: r.FormValue("email"), Phone: r.FormValue("phone"),
		Gender: r.FormValue("gender"), Country: r.FormValue("country"),
	}
	switch err := h.Users.Register(name, pass, prof); {
	case errors.Is(err, users.ErrInvalidProfile):
		// "invalid profile: email" -> reg_email
		fail(strings.ReplaceAll(strings.TrimPrefix(err.Error(), users.ErrInvalidProfile.Error()+": "), " ", ""))
		return
	case errors.Is(err, users.ErrInvalidUsername):
		fail("name")
		return
	case errors.Is(err, users.ErrWeakPassword):
		fail("weak")
		return
	case errors.Is(err, users.ErrExists), errors.Is(err, users.ErrEmailTaken), errors.Is(err, users.ErrReservedName):
		// One answer for a taken name, a taken email and a reserved name, so the
		// form does not tell a stranger which accounts or emails exist.
		fail("taken")
		return
	case err != nil:
		fail("off")
		return
	}
	audit.Record(name+" ("+ip+")", "user.register", name+" role=user email="+strings.ToLower(strings.TrimSpace(prof.Email)))
	// No automatic sign-in: send the new user to the login form.
	http.Redirect(w, r, "/?e=registered&as=user", http.StatusSeeOther)
}

// ResetQuota forgets every IP's sign-up count (used by tests).
func ResetQuota() {
	regMu.Lock()
	regSeen = map[string][]time.Time{}
	regMu.Unlock()
}
