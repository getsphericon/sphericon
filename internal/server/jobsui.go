package server

import (
	"database/sql"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"riverqueue.com/riverui"

	"github.com/getsphericon/sphericon/internal/paths"
)

// NewJobsUI builds the job queue's UI (River UI) under paths.StaffJobs, over the job queue's
// pool through an insert-only river client of its own: lists, retry, cancel, delete and queue
// pause. Its pages and assets are embedded in the binary. Job arguments are hidden in lists by
// default, since several kinds carry personal data. It must be started (Start) before it serves,
// and NewStaffAPI serves it to a signed-in Staff member only.
func NewJobsUI(db *sql.DB) (*riverui.Handler, error) {
	client, err := river.NewClient(riverdatabasesql.New(db), &river.Config{})
	if err != nil {
		return nil, err
	}
	return riverui.NewHandler(&riverui.HandlerOpts{
		Endpoints:                riverui.NewEndpoints(client, nil),
		JobListHideArgsByDefault: true,
		Logger:                   slog.Default(),
		Prefix:                   paths.StaffJobs,
	})
}
