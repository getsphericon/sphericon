// Package cleanup is for the errors of releasing something after the work is done: closing a
// connection, shutting a server down, removing a temporary file. The work has already succeeded or
// failed by then, so the error cannot change the outcome; it is logged instead of dropped.
package cleanup

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	// easement:disable-next-line external -- the shutdown report reads the container's errors (docs/architecture.md, Backend architecture)
	"github.com/samber/do/v2"
)

// Close closes c and logs a failure, naming what was closed.
func Close(c io.Closer, what string) {
	Report("close "+what, c.Close())
}

// Served logs why a server stopped serving, unless it was asked to stop (http.ErrServerClosed).
func Served(server string, err error) {
	if !errors.Is(err, http.ErrServerClosed) {
		Report("serve "+server, err)
	}
}

// Remove deletes the file at path and logs a failure; a file that is already gone is not one.
func Remove(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		Report("remove "+path, err)
	}
}

// Shutdown logs the services of a container that did not shut down cleanly.
func Shutdown(report *do.ShutdownReport) {
	if !report.Succeed {
		slog.Warn("cleanup failed", "step", "shut the container down", "error", report.Error())
	}
}

// Report logs err, if there is one, as a failed cleanup step.
func Report(step string, err error) {
	if err != nil {
		slog.Warn("cleanup failed", "step", step, "error", err)
	}
}
