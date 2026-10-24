package db_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/db"
)

func TestIsUniqueViolationRecognizesPostgresCode23505(t *testing.T) {
	assert.True(t, db.IsUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.True(t, db.IsUniqueViolation(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "23505"})))
	assert.False(t, db.IsUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, db.IsUniqueViolation(errors.New("plain")))
	assert.False(t, db.IsUniqueViolation(nil))
}

func TestIsUnavailableRecognizesALostOrOverloadedDatabase(t *testing.T) {
	for name, err := range map[string]error{
		"connect failure":     &pgconn.ConnectError{},
		"wrapped connect":     fmt.Errorf("begin: %w", &pgconn.ConnectError{}),
		"connection class 08": &pgconn.PgError{Code: "08006"},
		"shutting down":       &pgconn.PgError{Code: "57P01"},
		"crash shutdown":      &pgconn.PgError{Code: "57P02"},
		"cannot connect now":  &pgconn.PgError{Code: "57P03"},
		"database dropped":    &pgconn.PgError{Code: "57P04"},
		"idle session":        &pgconn.PgError{Code: "57P05"},
		"too many conns":      &pgconn.PgError{Code: "53300"},
		"bad conn":            driver.ErrBadConn,
		"conn done":           sql.ErrConnDone,
		"network":             &net.OpError{Op: "dial", Err: errors.New("refused")},
	} {
		assert.True(t, db.IsUnavailable(err), name)
	}
	for name, err := range map[string]error{
		"nil":              nil,
		"plain":            errors.New("plain"),
		"unique violation": &pgconn.PgError{Code: "23505"},
		"syntax error":     &pgconn.PgError{Code: "42601"},
		"query canceled":   &pgconn.PgError{Code: "57014"},
	} {
		assert.False(t, db.IsUnavailable(err), name)
	}
}
