package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const pgUniqueViolation = "23505"

// IsUniqueViolation reports whether err is a Postgres unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
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
		// 08 connection exception, 57P admin shutdown / crash / cannot connect now,
		// 53300 too many connections.
		return strings.HasPrefix(pgErr.Code, "08") || strings.HasPrefix(pgErr.Code, "57P") || pgErr.Code == "53300"
	default:
		return errors.As(err, &netErr)
	}
}
