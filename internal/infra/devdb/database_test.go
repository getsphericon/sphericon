package devdb_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/devdb"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SPHERICON_DATABASE_URL may be written in either form pgx reads, a URL or keyword/value pairs, and
// the database it names may be no plain identifier: upper case, a space and a double quote reach
// CREATE and DROP DATABASE as written.
func TestCreateAndDropTakeEitherDSNForm(t *testing.T) {
	forms := []struct {
		name string
		dsn  func(t *testing.T, databaseURL string) string
	}{
		{"url", func(_ *testing.T, databaseURL string) string { return databaseURL }},
		{"keyword/value", keywordValue},
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			absent := testhelper.AbsentDBURL(t, `Dev "devdb" db`)
			dsn := form.dsn(t, absent)
			cfg, err := pgx.ParseConfig(dsn)
			require.NoError(t, err)
			database := strconv.Quote(cfg.Database)

			created, err := devdb.Create(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, "database "+database+" created", created)
			reachable := ping(t, dsn)
			require.NoError(t, reachable, "the created database does not answer")

			again, err := devdb.Create(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, "database "+database+" already exists, skipping", again)

			dropped, err := devdb.Drop(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, "database "+database+" dropped", dropped)
			gone := ping(t, dsn)
			pgErr := new(pgconn.PgError)
			require.ErrorAs(t, gone, &pgErr)
			assert.Equal(t, pgerrcode.InvalidCatalogName, pgErr.Code)
		})
	}
}

// keywordValue is the URL databaseURL written as keyword/value pairs, its query parameters
// included, each value quoted.
func keywordValue(t *testing.T, databaseURL string) string {
	t.Helper()
	u, err := url.Parse(databaseURL)
	require.NoError(t, err)
	password, _ := u.User.Password()
	pairs := []string{
		"host=" + quoted(u.Hostname()),
		"port=" + quoted(u.Port()),
		"user=" + quoted(u.User.Username()),
		"password=" + quoted(password),
		"dbname=" + quoted(strings.TrimPrefix(u.Path, "/")),
	}
	for key, values := range u.Query() {
		for _, value := range values {
			pairs = append(pairs, key+"="+quoted(value))
		}
	}
	return strings.Join(pairs, " ")
}

// quoted is value as a quoted keyword/value value: a backslash and a single quote escaped.
func quoted(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return fmt.Sprintf("'%s'", escaped)
}

// ping connects to the database dsn names and closes the connection again, so it holds up no
// DROP DATABASE after it.
func ping(t *testing.T, dsn string) error {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer cleanup.Close(db, "ping")
	return db.PingContext(t.Context())
}
