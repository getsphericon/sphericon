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

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

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

func TestIsConnectionLostRecognizesAConnectionClosedMidRequest(t *testing.T) {
	assert.True(t, db.IsConnectionLost(pgconn.ErrConnClosed))
	assert.True(t, db.IsConnectionLost(fmt.Errorf("exec: %w", pgconn.ErrConnClosed)))
	assert.False(t, db.IsConnectionLost(nil))
	assert.False(t, db.IsConnectionLost(&pgconn.ConnectError{}))
	assert.False(t, db.IsUnavailable(pgconn.ErrConnClosed), "the request may have taken effect")
}

// A row ent did not find is the module's own refusal; any other error, and no error, pass as they
// are.
func TestNotFoundAsTurnsOnlyAMissIntoTheRefusal(t *testing.T) {
	refusal := errors.New("the row is not there")
	other := errors.New("connection reset")
	miss := fmt.Errorf("get: %w", &ent.NotFoundError{})

	missed := db.NotFoundAs(miss, refusal)
	failed := db.NotFoundAs(other, refusal)
	found := db.NotFoundAs(nil, refusal)

	assert.Equal(t, refusal, missed)
	assert.Equal(t, other, failed)
	assert.NoError(t, found)
}
