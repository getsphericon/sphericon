package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/pressly/goose/v3/lock"

	"github.com/getsphericon/sphericon/internal/infra/postgres"
)

// A migration that finds the migration lock taken reports who holds it: first after
// lockReportFirst, about when goose's first retry finds the lock still taken, then every
// lockReportEvery, so the wait does not look like a hang. Once it takes the lock it says so.
const (
	lockReportFirst = time.Second
	lockReportEvery = 10 * time.Second
	// lockReportTimeout bounds one read of the holders: the report never holds up the migration.
	lockReportTimeout = 5 * time.Second
)

// migrationLockID is the advisory lock key the migrations take: goose's default.
const migrationLockID = lock.DefaultLockID

// reportingLocker is goose's session locker with the wait for it reported: it takes and releases
// the lock exactly as the locker it wraps does, and reads the lock's holders through sqlDB, never
// through the connection that waits for the lock. With a pool of one connection the reads find no
// connection, and the wait goes unreported.
type reportingLocker struct {
	lock.SessionLocker

	sqlDB *sql.DB
}

func (l reportingLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	// The waiting session is left out of the holders: the lock it has just taken is no wait.
	waiterPID, err := postgres.BackendPID(ctx, conn)
	if err != nil {
		return fmt.Errorf("take the migration lock: %w", err)
	}
	waitCtx, stop := context.WithCancel(ctx)
	reportDone := make(chan bool, 1)
	go func() { reportDone <- l.reportHolders(waitCtx, waiterPID) }()
	err = l.SessionLocker.SessionLock(ctx, conn)
	stop()
	reportedAWait := <-reportDone
	if reportedAWait && err == nil {
		slog.InfoContext(ctx, "took the migration lock")
	}
	return err
}

// reportHolders logs who other than the session waiterPID holds the migration lock on its
// schedule until ctx ends, and tells whether it logged a holder.
func (l reportingLocker) reportHolders(ctx context.Context, waiterPID int) bool {
	reported := false
	waited := lockReportFirst
	timer := time.NewTimer(lockReportFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return reported
		case <-timer.C:
		}
		// The next report is due an interval after this one is, however long this read takes.
		timer.Reset(lockReportEvery)
		holders, err := l.holders(ctx, waiterPID)
		waitEnded := ctx.Err() != nil // a read cut short by the lock being taken is no failure
		if err != nil && !waitEnded {
			slog.WarnContext(ctx, "a migration waits for the migration lock and cannot read who holds it", "waited", waited, "error", err)
		}
		for _, h := range holders {
			slog.WarnContext(ctx, "a migration waits for the migration lock another session holds",
				"waited", waited, "holder_pid", h.PID, "holder_application", h.Application,
				"holder_client_addr", h.ClientAddr, "holder_backend_start", h.BackendStart)
			reported = true
		}
		waited += lockReportEvery
	}
}

func (l reportingLocker) holders(ctx context.Context, waiterPID int) ([]postgres.LockHolder, error) {
	ctx, cancel := context.WithTimeout(ctx, lockReportTimeout)
	defer cancel()
	return postgres.LockHolders(ctx, l.sqlDB, migrationLockID, waiterPID)
}
