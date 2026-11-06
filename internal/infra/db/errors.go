package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation reports whether err is a Postgres unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

// IsUnavailable reports whether err means the database cannot be reached right now: a
// failed or dropped connection, a server shutting down or out of connections. The
// request that met it changed nothing and a retry can succeed, unlike any other error.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var connect *pgconn.ConnectError
	var pgErr *pgconn.PgError
	var netErr net.Error
	switch {
	case errors.As(err, &connect), errors.Is(err, driver.ErrBadConn), errors.Is(err, sql.ErrConnDone):
		return true
	case errors.As(err, &pgErr):
		// Class 08 connection exception, a server shutting down, restarting or dropping the
		// session (57P01 to 57P05, the operator-intervention codes that end a connection),
		// too many connections. A query_canceled (57014) is a statement timeout or a cancelled
		// context, not an unreachable database, and is not listed.
		switch pgErr.Code {
		case pgerrcode.AdminShutdown, pgerrcode.CrashShutdown, pgerrcode.CannotConnectNow,
			pgerrcode.DatabaseDropped, pgerrcode.IdleSessionTimeout, pgerrcode.TooManyConnections:
			return true
		}
		return pgerrcode.IsConnectionException(pgErr.Code)
	default:
		return errors.As(err, &netErr)
	}
}
