package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"velexa-sim-me/internal/config"
)

// Healthcheck asks the engine on this machine for /healthz and returns the
// process exit code: 0 healthy, 1 not. Docker's HEALTHCHECK runs it, so the
// image needs no curl or wget.
func Healthcheck() int {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + config.EnvOr("PORT", "8010") + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: HTTP", resp.StatusCode)
		return 1
	}
	return 0
}
