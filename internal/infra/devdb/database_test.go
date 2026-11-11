package devdb_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/devdb"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SPHERICON_DATABASE_URL may be written in either form pgx reads, a URL or keyword/value pairs, and
// the database it names may be no plain identifier: upper case, a space and a double quote reach
// CREATE and DROP DATABASE as written.
func TestCreateAndDropTakeEitherDSNForm(t *testing.T) {
	forms := []struct {
		name string
		dsn  func(server *pgx.ConnConfig, database string) string
	}{
		{"url", func(server *pgx.ConnConfig, database string) string {
			return (&url.URL{
				Scheme:   "postgres",
				User:     url.UserPassword(server.User, server.Password),
				Host:     net.JoinHostPort(server.Host, strconv.Itoa(int(server.Port))),
				Path:     "/" + database,
				RawQuery: "sslmode=disable",
			}).String()
		}},
		{"keyword/value", func(server *pgx.ConnConfig, database string) string {
			return fmt.Sprintf("host=%s port=%d user=%s password='%s' dbname='%s' sslmode=disable",
				server.Host, server.Port, server.User, server.Password, database)
		}},
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			// The test's scratch database gives the server's address and a name no other test takes.
			scratch, err := pgx.ParseConfig(testhelper.EmptyDBURL(t))
			require.NoError(t, err)
			database := `Dev "` + scratch.Database + `" db`
			dsn := form.dsn(scratch, database)
			// The database is the one under test, so no helper of the harness owns it: it is
			// dropped here should the test stop before its own Drop (t.Context is done by then).
			t.Cleanup(func() {
				_, err := devdb.Drop(context.WithoutCancel(t.Context()), dsn)
				assert.NoError(t, err, "drop %s", database)
			})

			created, err := devdb.Create(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("database %s created", strconv.Quote(database)), created)
			assertReachable(t, dsn)

			again, err := devdb.Create(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("database %s already exists, skipping", strconv.Quote(database)), again)

			dropped, err := devdb.Drop(t.Context(), dsn)

			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("database %s dropped", strconv.Quote(database)), dropped)
			assert.Error(t, ping(t, dsn), "the dropped database still answers")
		})
	}
}

func assertReachable(t *testing.T, dsn string) {
	t.Helper()
	require.NoError(t, ping(t, dsn), "the created database does not answer")
}

// ping connects to the database dsn names and closes the connection again, so it holds up no
// DROP DATABASE after it.
func ping(t *testing.T, dsn string) error {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	return db.PingContext(t.Context())
}
