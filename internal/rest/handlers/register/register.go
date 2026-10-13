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
	maxRegistrations  = 5 // per client IP per window
	registrationSpan  = time.Hour
	registerErrPrefix = "reg_"
)

var (
	regMu   sync.Mutex
	regSeen = map[string][]time.Time{}
)

// registrationAllowed reports whether the IP still has sign-ups left this hour.
func registrationAllowed(ip string) bool {
	regMu.Lock()
	defer regMu.Unlock()
	cutoff := time.Now().Add(-registrationSpan)
	kept := regSeen[ip][:0]
	for _, t := range regSeen[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	regSeen[ip] = kept
	return len(kept) < maxRegistrations
}

// noteRegistration counts a successful sign-up against the IP's quota.
func noteRegistration(ip string) {
	regMu.Lock()
	regSeen[ip] = append(regSeen[ip], time.Now())
	regMu.Unlock()
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
	r.ParseForm()
	if !registrationAllowed(middleware.ClientIP(r)) {
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
	case errors.Is(err, users.ErrEmailTaken):
		fail("emailtaken")
		return
	case errors.Is(err, users.ErrInvalidUsername):
		fail("name")
		return
	case errors.Is(err, users.ErrWeakPassword):
		fail("weak")
		return
	case errors.Is(err, users.ErrExists):
		fail("exists")
		return
	case err != nil:
		fail("off")
		return
	}
	noteRegistration(middleware.ClientIP(r))
	audit.Record(name+" ("+middleware.ClientIP(r)+")", "user.register", name+" role=user email="+strings.ToLower(strings.TrimSpace(prof.Email)))
	// No automatic sign-in: send the new user to the login form.
	http.Redirect(w, r, "/?e=registered&as=user", http.StatusSeeOther)
}

// ResetQuota forgets every IP's sign-up count (used by tests).
func ResetQuota() {
	regMu.Lock()
	regSeen = map[string][]time.Time{}
	regMu.Unlock()
}
