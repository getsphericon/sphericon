package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/getsphericon/sphericon/ent"
)

// IsUnavailable reports whether err means the database cannot be reached right now: a
// failed or dropped connection, a server shutting down or out of connections. The
// request that met it changed nothing and a retry can succeed, unlike any other error.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	_, connect := errors.AsType[*pgconn.ConnectError](err)
	pgErr, fromServer := errors.AsType[*pgconn.PgError](err)
	switch {
	case connect, errors.Is(err, driver.ErrBadConn), errors.Is(err, sql.ErrConnDone):
		return true
	case fromServer:
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
		_, network := errors.AsType[net.Error](err)
		return network
	}
}

// IsConnectionLost reports whether err means the connection went away in the middle of a
// request, after its statement may have reached the server: unlike IsUnavailable, whether the
// request took effect is unknown, so only a request whose retry is idempotent may be retried.
func IsConnectionLost(err error) bool {
	return errors.Is(err, pgconn.ErrConnClosed)
}

// NotFoundAs is refusal when err is ent's miss of a row, and err otherwise: a module answers a row
// it was asked for and does not hold with its own not-found refusal, as Constraints answers a
// violated constraint with the refusal declared for it.
func NotFoundAs(err, refusal error) error {
	if ent.IsNotFound(err) {
		return refusal
	}
	return err
}
