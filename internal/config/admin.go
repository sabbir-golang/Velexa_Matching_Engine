package config

import (
	"os"
	"strings"
)

// AdminUser is the console username that always works as an admin (ME_ADMIN_USER).
func AdminUser() string { return EnvOr("ME_ADMIN_USER", "admin") }

// AdminPass is that account's password (ME_ADMIN_PASS).
func AdminPass() string { return EnvOr("ME_ADMIN_PASS", "Velexa@ME2026") }

// AdminSecret is the key that signs session cookies (ME_ADMIN_SECRET).
func AdminSecret() string { return EnvOr("ME_ADMIN_SECRET", "velexa-me/"+AdminPass()) }

// RegistrationOpen reports whether self-registration is on (ME_REGISTRATION=off turns it off).
func RegistrationOpen() bool {
	return strings.ToLower(os.Getenv("ME_REGISTRATION")) != "off"
}
