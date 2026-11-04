//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

// env is the shared application and Mailpit; each test isolates itself with env.NewWorkspace.
var env *Env

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	e, stop, err := Boot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: boot:", err)
		return 1
	}
	defer stop()
	env = e
	code := m.Run()
	if os.Getenv("SPHERICON_E2E_HOLD") == "true" {
		hold(e)
	}
	return code
}

// hold keeps the application and Mailpit running after the run (SPHERICON_E2E_HOLD=true) so a failed
// scenario can be inspected in the Mailpit UI; SIGINT or SIGTERM stops it.
func hold(e *Env) {
	fmt.Fprintf(os.Stderr, "e2e: SPHERICON_E2E_HOLD=true, the stack stays up\n  application: %s\n  mailpit:     %s\n  Ctrl-C to stop\n", e.BaseURL, e.mailpit.URL())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
