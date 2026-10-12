package db_test

import (
	"errors"
	"fmt"
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
