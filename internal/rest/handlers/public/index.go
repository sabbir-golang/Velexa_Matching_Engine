package public

import (
	"net/http"
	"strings"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/web"
)

// Index serves /.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, embedded := r.URL.Query()["embed"]; !embedded {
		h.Welcome(w, r)
		return
	}
	h.Terminal(w, r)
}

// Terminal serves the Trade Terminal (also embedded in the console as /?embed=1).
func (h *Handler) Terminal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(web.IndexHTML))
}

// Welcome serves the landing page at / with the user and admin sign-in.
func (h *Handler) Welcome(w http.ResponseWriter, r *http.Request) {
	if u := middleware.SessionUser(h.Users, r); u != nil {
		http.Redirect(w, r, middleware.HomeFor(u), http.StatusSeeOther)
		return
	}
	msg, msgClass := "", "err"
	switch r.URL.Query().Get("e") {
	case "1":
		msg = "Invalid username or password."
	case "lock":
		msg = "Too many attempts. Try again in a few minutes."
	case "registered":
		msg = "Account created. Sign in with your username and password."
		msgClass = "ok"
	case "reg_off":
		msg = "Registration is closed. Ask an administrator for an account."
	case "reg_rate":
		msg = "Too many sign-ups from this address. Try again later."
	case "reg_mismatch":
		msg = "The two passwords do not match."
	case "reg_name":
		msg = "Username must be 3-32 characters: letters, digits, . _ -"
	case "reg_weak":
		msg = "Password must be 10-128 characters and use at least 3 of: lower-case, upper-case, digits, symbols."
	case "reg_fullname":
		msg = "Enter your full name."
	case "reg_email":
		msg = "Enter a valid email address."
	case "reg_phone":
		msg = "Enter a valid phone number (7-15 digits, may start with +)."
	case "reg_gender":
		msg = "Select a gender."
	case "reg_country":
		msg = "Select a country."
	case "reg_taken":
		msg = "An account could not be created with these details. Choose another username, or sign in if you already have an account."
	case "csrf":
		msg = "The form expired. Please try again."
	case "admin":
		msg = "This is an administrator account. Use Admin login."
	case "role":
		msg = "This account is not an administrator. Use User login."
	}
	as := "user"
	switch r.URL.Query().Get("as") {
	case "admin":
		as = "admin"
	case "register":
		if config.RegistrationOpen() {
			as = "register"
		}
	}
	reg := "1"
	if !config.RegistrationOpen() {
		reg = "0"
	}
	page := strings.Replace(web.WelcomeHTML, "__ERR__", msg, 1)
	page = strings.Replace(page, "__ERRCLASS__", msgClass, 1)
	page = strings.Replace(page, "__AS__", as, -1)
	page = strings.Replace(page, "__REG__", reg, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}
