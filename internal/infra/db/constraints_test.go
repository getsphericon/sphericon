package db_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const tagName = "tags_workspace_id_name"

var errTagTaken = errors.New("tag taken")

// tags declares the one unique constraint of the tags table a refusal.
var tags = db.Declare(db.Refusal(tagName, errTagTaken))

func newBus(sqlDB *sql.DB) *events.Bus {
	return events.New(sqlDB, func(d dialect.Driver) *ent.Client { return ent.NewClient(ent.Driver(d)) }, time.Now)
}

func poolClient(sqlDB *sql.DB) *ent.Client {
	return ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
}

func addTag(t *testing.T, c *ent.Client, name string) func() error {
	t.Helper()
	return func() error {
		// test writes: a Tag whose name the constraint under test refuses or lets through.
		return c.Tag.Create().SetWorkspaceID(fixtures.AcmeID).SetName(name).Exec(t.Context())
	}
}

func TestDeclareRecordsTheDeclaringPackage(t *testing.T) {
	declarations := db.Declarations()

	assert.Contains(t, declarations, db.Declaration{
		Owner: "github.com/getsphericon/sphericon/internal/infra/db_test", Constraint: tagName, Refusal: errTagTaken,
	})
}

// A refusal inside a transaction someone else opened leaves that transaction usable: the next
// write of the same transaction commits.
func TestWriteRefusesUnderASavepointOfTheCallersTransaction(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	bus := newBus(sqlDB)

	var refused error
	err := bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		refused = tags.Write(t.Context(), tx, addTag(t, tx, fixtures.TagNewsletterName))
		return tags.Write(t.Context(), tx, addTag(t, tx, "after-the-refusal"))
	})

	require.NoError(t, err)
	require.ErrorIs(t, refused, errTagTaken)
	exists, err := poolClient(sqlDB).Tag.Query().
		Where(tag.WorkspaceID(fixtures.AcmeID), tag.Name("after-the-refusal")).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, exists, "the write after the refusal committed")
}

// On the pool there is no transaction to keep: the write runs as it is and its refusal is read.
func TestWriteRefusesOnThePool(t *testing.T) {
	c := poolClient(testhelper.RealDB(t))

	refused := tags.Write(t.Context(), c, addTag(t, c, fixtures.TagNewsletterName))
	next := tags.Write(t.Context(), c, addTag(t, c, "next-on-the-pool"))

	require.ErrorIs(t, refused, errTagTaken)
	require.NoError(t, next)
}

// What is not a declared refusal goes up as it is: a constraint declared by no one, a foreign key,
// a CHECK.
func TestWriteReturnsAnyOtherViolationUnchanged(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	bus := newBus(sqlDB)
	undeclared := db.Declare()

	cases := map[string]struct {
		constraints db.Constraints
		code        string
		write       func(tx *ent.Client) error
	}{
		"an undeclared unique violation": {undeclared, pgerrcode.UniqueViolation, func(tx *ent.Client) error {
			return addTag(t, tx, fixtures.TagNewsletterName)()
		}},
		"a foreign key violation": {tags, pgerrcode.ForeignKeyViolation, func(tx *ent.Client) error {
			// test writes: a Tag of a Workspace that does not exist, which the foreign key refuses.
			return tx.Tag.Create().SetWorkspaceID(999999).SetName("orphan").Exec(t.Context())
		}},
		"a check violation": {tags, pgerrcode.CheckViolation, func(tx *ent.Client) error {
			// test writes: a Contact whose address the lowercase CHECK refuses.
			return tx.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("Upper@Example.com").Exec(t.Context())
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var got error
			err := bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
				got = c.constraints.Write(t.Context(), tx, func() error { return c.write(tx) })
				return nil
			})
			require.NoError(t, err)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, got, &pgErr)
			assert.Equal(t, c.code, pgErr.Code)
		})
	}
}

// An insert ON CONFLICT DO NOTHING is written, or skipped when its row is there, which ent reports
// as sql.ErrNoRows; any other error goes up as it is.
func TestInsertedTellsAWrittenRowFromASkippedOne(t *testing.T) {
	c := poolClient(testhelper.RealDB(t))
	insert := func(name string) error {
		// test writes: a Tag inserted ON CONFLICT DO NOTHING, written or skipped as the case says.
		_, err := c.Tag.Create().SetWorkspaceID(fixtures.AcmeID).SetName(name).
			OnConflict(entsql.ConflictColumns(tag.FieldWorkspaceID, tag.FieldName)).DoNothing().ID(t.Context())
		return err
	}
	failure := errors.New("the insert failed")

	written, err := db.Inserted(insert("a-new-tag"))
	require.NoError(t, err)
	skipped, skippedErr := db.Inserted(insert(fixtures.TagNewsletterName))
	require.NoError(t, skippedErr)
	failed, failedErr := db.Inserted(failure)

	assert.True(t, written)
	assert.False(t, skipped)
	assert.False(t, failed)
	assert.ErrorIs(t, failedErr, failure)
}
