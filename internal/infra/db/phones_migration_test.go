package db_test

import (
	"io/fs"
	"strconv"
	"testing"
	"testing/fstest"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Instance upgraded from before ADR 0075 has its phones rewritten to E.164: a local number read
// with the Contact's country, a value that does not parse dropped, and a number two Contacts of a
// Workspace share kept by one of them, so the unique index never fails the migration.
func TestMigrateRewritesPhonesToE164(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.MigrateFS(t.Context(), sqlDB, migrationsBefore(t, db.PhonesToE164Version))
	require.NoError(t, err)
	acme := testhelper.InsertLegacyWorkspace(t, sqlDB, "acme0001")
	globex := testhelper.InsertLegacyWorkspace(t, sqlDB, "globex01")
	contactWith := func(ws int64, phone string, country *string) int64 {
		return testhelper.InsertLegacyContact(t, sqlDB, ws, phone, country)
	}
	international := contactWith(acme, "+1 (415) 555-0100", nil)
	local := contactWith(acme, "030 1234567", new("DE"))
	garbage := contactWith(acme, "call me", nil)
	localNoCountry := contactWith(acme, "415 555 0101", nil)
	olderTwin := contactWith(acme, "(415) 555-0102", new("US"))
	youngerTwin := contactWith(acme, "+1 415-555-0102", nil)
	stranger := contactWith(acme, "8 999 123-45-67", new("RU"))
	storedFirst := contactWith(acme, "+79991234567", nil)
	otherWorkspace := contactWith(globex, "+1 (415) 555-0100", nil)

	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	t.Cleanup(func() { cleanup.Close(client, "ent client") })
	phones := phonesByID(t, client)
	assert.Equal(t, map[int64]string{
		international:  "+14155550100",
		local:          "+49301234567",
		olderTwin:      "+14155550102",
		storedFirst:    "+79991234567",
		otherWorkspace: "+14155550100",
	}, phones, "the oldest twin keeps a shared number, unless another already stores it in E.164")
	for _, id := range []int64{garbage, localNoCountry, youngerTwin, stranger} {
		assert.NotContains(t, phones, id)
	}
}

// phonesByID is the phone of each Contact that has one.
func phonesByID(t *testing.T, client *ent.Client) map[int64]string {
	t.Helper()
	all := client.Contact.Query().Where(contact.PhoneNotNil()).AllX(t.Context())
	return lo.SliceToMap(all, func(c *ent.Contact) (int64, string) { return c.ID, *c.Phone })
}

// migrationsBefore is the embedded SQL migrations older than version.
func migrationsBefore(t *testing.T, version int64) fstest.MapFS {
	t.Helper()
	all, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	require.NoError(t, err)
	entries, err := fs.ReadDir(all, ".")
	require.NoError(t, err)
	out := fstest.MapFS{}
	for _, e := range entries {
		if e.Name() < strconv.FormatInt(version, 10) {
			data, err := fs.ReadFile(all, e.Name())
			require.NoError(t, err)
			out[e.Name()] = &fstest.MapFile{Data: data}
		}
	}
	return out
}
