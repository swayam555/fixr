package platform

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// `<binary> -healthcheck` probes the running instance's /healthz and exits 0 or 1, so container
// healthchecks work on distroless images that have no shell or curl. Needs ADDR set.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "-healthcheck" {
		return
	}
	addr := os.Getenv("ADDR")
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get("http://" + addr + "/healthz")
	if err != nil || res.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
