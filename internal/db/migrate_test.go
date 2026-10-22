package db_test

import (
	"database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/refsettle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

func openEmpty(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// Two migrations started together run one after the other: the second finds nothing to do.
func TestMigrateStartedTwiceTogetherRunsOnceAndTheSecondFindsNothing(t *testing.T) {
	sqlDB := openEmpty(t)

	var wg sync.WaitGroup
	applied := make([]int, 2)
	errs := make([]error, 2)
	for i := range applied {
		wg.Go(func() { applied[i], errs[i] = db.Migrate(t.Context(), sqlDB) })
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Zero(t, min(applied[0], applied[1]))
	assert.Positive(t, max(applied[0], applied[1]))

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
	assert.Equal(t, st.Newest, st.Current)
}

// Status reads the schema and changes nothing.
func TestStatusOnAnEmptyDatabaseListsEveryMigrationAndLeavesItEmpty(t *testing.T) {
	sqlDB := openEmpty(t)

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Zero(t, st.Current)
	assert.Positive(t, st.Newest)
	assert.NotEmpty(t, st.Pending)

	again, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, st, again)

	applied, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Len(t, st.Pending, applied)
}

// Migrate has no time limit of its own: it applies a migration under a context with no
// deadline, and the lock wait is bounded by that context alone.
func TestMigrateFSAppliesAMigrationUnderAContextWithNoDeadline(t *testing.T) {
	sqlDB := openEmpty(t)
	files := fstest.MapFS{"20200101000000_slow.sql": {Data: []byte("-- +goose Up\nCREATE TABLE slow_done (id int);\n")}}
	_, hasDeadline := t.Context().Deadline()
	require.False(t, hasDeadline)

	applied, err := db.MigrateFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)

	st, err := db.StatusFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
}

// An Instance upgraded from before the Workspace id gets one for every Workspace it already had,
// and the column ends required and unique (ADR 0043).
func TestMigrateGivesExistingWorkspacesAnIdAndEndsRequired(t *testing.T) {
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < "20261020030001" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261020030001_workspace_public_id.sql"))

	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	t.Cleanup(func() { _ = client.Close() })
	workspaces, err := client.Workspace.Query().All(t.Context())
	require.NoError(t, err)
	require.Len(t, workspaces, 3)
	seen := map[string]bool{}
	for _, ws := range workspaces {
		assert.Regexp(t, workspaceid.Pattern, ws.PublicID)
		seen[ws.PublicID] = true
	}
	assert.Len(t, seen, 3, "every id is its own")

	// The ids are unique now: a fourth Workspace cannot take one of them.
	// test writes: the unique violation is the outcome under test
	err = client.Workspace.Create().SetName("y").SetCollectKey("y").SetPublicID(workspaces[0].PublicID).Exec(t.Context())
	assert.True(t, db.IsUniqueViolation(err))
}

// Dropping the slug refuses an Instance whose Workspaces have not all got their id yet: it would
// leave them with no address (ADR 0033 guard).
func TestMigrateRefusesToDropTheSlugBeforeEveryWorkspaceHasAnId(t *testing.T) {
	sqlDB := openEmpty(t)
	// Skip the id backfill and the migration that requires the id.
	skipping := migrationsWhere(t, func(name string) bool { return name < "20261020030002" || name >= "20261020030004" })
	before := migrationsWhere(t, func(name string) bool { return name < "20261020030002" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261020030001_workspace_public_id.sql"))

	_, err = db.MigrateFS(t.Context(), sqlDB, skipping)
	require.ErrorContains(t, err, "workspaces.slug cannot be dropped")
}

// legacyFixtures is the directory of the fixtures that belong to one migration: the rows of the schema
// just before it, which the ent client cannot write (see testhelper.LoadLegacyFixtures).
func legacyFixtures(migration string, parts ...string) string {
	return filepath.Join(append([]string{"testdata", "legacy", strings.TrimSuffix(migration, ".sql")}, parts...)...)
}

// migrationsWhere is the embedded migrations whose file name satisfies keep.
func migrationsWhere(t *testing.T, keep func(name string) bool) fstest.MapFS {
	t.Helper()
	all, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	require.NoError(t, err)
	entries, err := fs.ReadDir(all, ".")
	require.NoError(t, err)
	out := fstest.MapFS{}
	for _, e := range entries {
		if keep(e.Name()) {
			data, err := fs.ReadFile(all, e.Name())
			require.NoError(t, err)
			out[e.Name()] = &fstest.MapFile{Data: data}
		}
	}
	return out
}

// Dropping the recipient's own outcome refuses a Broadcast recipient that was decided before its
// Outbound message existed: the outcome would be lost with the column (ADR 0033 guard). A pending
// recipient has nothing to lose, and once the message exists the migration goes through.
func TestMigrateRefusesToDropARecipientOutcomeThatHasNoOutboundMessage(t *testing.T) {
	const dropping = "20261020185327_drop_recipient_outcome_and_broadcast_counters.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < dropping })
	all := migrationsWhere(t, func(name string) bool { return name <= dropping })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(dropping, "decided"))

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.ErrorContains(t, err, "would lose their outcome")

	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(dropping, "sent_message"))
	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	columns := testhelper.ColumnsOf(t, sqlDB, "broadcast_recipients", "status", "error", "sent_at", "outbound_message_id")
	assert.Zero(t, columns, "the recipient's own outcome is gone")
}

// Replacing the Sending domain flag by moments keeps what the flag said: a domain verified now stays
// verified, one that was verified once and is not stays unverified with a loss moment (the drop
// alone would turn it verified), and one never verified has no moment at all.
func TestMigrateCarriesTheSendingDomainFlagIntoMoments(t *testing.T) {
	const replacing = "20261021193611_sending_domain_verification_moments.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < replacing })
	all := migrationsWhere(t, func(string) bool { return true })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(replacing))

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	domains, err := client.SendingDomain.Query().All(t.Context())
	require.NoError(t, err)
	moments := map[string][2]bool{}
	for _, d := range domains {
		moments[d.Domain] = [2]bool{d.VerifiedAt != nil, d.VerificationLostAt != nil}
	}
	assert.Equal(t, map[string][2]bool{
		"live.test":  {true, false},
		"lost.test":  {true, true},
		"never.test": {false, false},
	}, moments)
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "sending_domains", "verified"), "the flag is gone")
}

// Replacing the Webhook endpoint flag by disabled_at keeps what the flag said: an enabled endpoint
// stays enabled, one an owner turned off stays off with the reason "owner", and one the worker
// turned off keeps its reason.
func TestMigrateCarriesTheWebhookEndpointFlagIntoDisabledAt(t *testing.T) {
	const replacing = "20261022000001_webhook_endpoint_disabled_at.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < replacing })
	all := migrationsWhere(t, func(string) bool { return true })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(replacing))

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	endpoints, err := client.WebhookEndpoint.Query().All(t.Context())
	require.NoError(t, err)
	off := map[string]bool{}
	reasons := map[string]*webhookendpoint.DisabledReason{}
	for _, e := range endpoints {
		off[e.URL] = e.DisabledAt != nil
		reasons[e.URL] = e.DisabledReason
	}
	assert.Equal(t, map[string]bool{"https://on.test": false, "https://owner.test": true, "https://gone.test": true}, off)
	assert.Equal(t, map[string]*webhookendpoint.DisabledReason{
		"https://on.test":    nil,
		"https://owner.test": lo.ToPtr(webhookendpoint.DisabledReasonOwner),
		"https://gone.test":  lo.ToPtr(webhookendpoint.DisabledReasonGone),
	}, reasons)
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "webhook_endpoints", "enabled"), "the flag is gone")
}

// Replacing the Usage flag by finalized_at keeps what the flag said: a final row stays final, with the
// moment it was last computed, and Live usage stays unset.
func TestMigrateCarriesTheUsageFlagIntoFinalizedAt(t *testing.T) {
	const replacing = "20261022115603_usage_finalized_at.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < replacing })
	all := migrationsWhere(t, func(string) bool { return true })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(replacing))

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	rows, err := client.Usage.Query().All(t.Context())
	require.NoError(t, err)
	finalizedAt := map[string]string{}
	for _, r := range rows {
		finalizedAt[r.PeriodStart.UTC().Format("2006-01")] = lo.FromPtr(r.FinalizedAt).UTC().Format("2006-01-02 15:04")
	}
	assert.Equal(t, map[string]string{
		"2025-12": "2026-01-01 00:05",
		"2026-01": "0001-01-01 00:00",
	}, finalizedAt, "the closed period carries its computed moment, the open one none")
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "usages", "final"), "the flag is gone")
}

// Replacing the Integration quota flag by provider_quota_failed_at keeps what the flag said: a
// healthy Integration stays unset, a failing one gets its last check as the moment (or its last update
// when it has no check).
func TestMigrateCarriesTheIntegrationQuotaFlagIntoFailedAt(t *testing.T) {
	const replacing = "20261022120110_integration_quota_failed_at.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < replacing })
	all := migrationsWhere(t, func(string) bool { return true })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures(replacing))

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	rows, err := client.Integration.Query().All(t.Context())
	require.NoError(t, err)
	failedAt := map[string]string{}
	for _, r := range rows {
		failedAt[r.Name] = lo.FromPtr(r.ProviderQuotaFailedAt).UTC().Format("2006-01-02 15:04")
	}
	assert.Equal(t, map[string]string{
		"ok":        "0001-01-01 00:00",
		"checked":   "2026-02-02 10:00",
		"unchecked": "2026-01-03 10:00",
	}, failedAt)
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "integrations", "provider_quota_unavailable"), "the flag is gone")
}

// Dropping automations.definition refuses an Instance whose Automations have not all been given a
// version yet (the river job backfill_automation_versions): the steps would be lost (ADR 0049).
func TestMigrateRefusesToDropTheDefinitionBeforeTheBackfill(t *testing.T) {
	sqlDB := openEmpty(t)
	legacy := migrationsWhere(t, func(name string) bool { return name < "20261020210001" })
	_, err := db.MigrateFS(t.Context(), sqlDB, legacy)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261020210003_drop_automation_definition.sql"))
	all := migrationsWhere(t, func(string) bool { return true })
	beforeDrop := migrationsWhere(t, func(name string) bool { return name < "20261020210003" })
	_, err = db.MigrateFS(t.Context(), sqlDB, beforeDrop)
	require.NoError(t, err)

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.ErrorContains(t, err, "automations.definition cannot be dropped")
	assert.Equal(t, 1, testhelper.ColumnsOf(t, sqlDB, "automations", "definition"), "the column is still there")
}

// An Instance whose Outbound messages hold ids of rows that are gone (an Automation run or a Template
// erased before the references were foreign keys) migrates: the keys are added NOT VALID, so the
// old rows are left alone and the next write to one still works.
func TestMigrateAddsTheOutboundMessageKeysNotValidOverDanglingIds(t *testing.T) {
	sqlDB := openEmpty(t)
	client := migrateWithDanglingOutboundRefs(t, sqlDB)

	assert.Len(t, testhelper.UnvalidatedConstraints(t, sqlDB, "outbound_messages"), 6)
	err := client.OutboundMessage.UpdateOneID(2).SetStatus(outboundmessage.StatusFailed).Exec(t.Context())
	require.NoError(t, err, "a message with a dangling id can still change")
}

// The job clears exactly the ids that point at nothing, keeps every other column and every id that
// resolves, then validates the keys; once it has run it finds nothing.
func TestClearOutboundMessageRefsClearsDanglingIdsThenValidatesTheKeys(t *testing.T) {
	sqlDB := openEmpty(t)
	client := migrateWithDanglingOutboundRefs(t, sqlDB)

	// A batch of one makes the job loop.
	cleared, err := refsettle.New(client).ClearOutboundMessageRefs(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, cleared)

	type refs struct {
		Template, Run, Integration *int64
		Destination                string
	}
	messages, err := client.OutboundMessage.Query().Order(ent.Asc(outboundmessage.FieldID)).All(t.Context())
	require.NoError(t, err)
	got := lo.Map(messages, func(m *ent.OutboundMessage, _ int) refs {
		return refs{m.TemplateID, m.AutomationRunID, m.IntegrationID, m.Destination}
	})
	assert.Equal(t, []refs{
		{new(int64(1)), nil, nil, "a@x.test"},
		{nil, nil, nil, "b@x.test"},
		{nil, nil, nil, "c@x.test"},
	}, got)
	assert.Empty(t, testhelper.UnvalidatedConstraints(t, sqlDB, "outbound_messages"))

	cleared, err = refsettle.New(client).ClearOutboundMessageRefs(t.Context(), 1)
	require.NoError(t, err)
	assert.Zero(t, cleared)
}

func migrateWithDanglingOutboundRefs(t *testing.T, sqlDB *sql.DB) *ent.Client {
	t.Helper()
	before := migrationsWhere(t, func(name string) bool { return name < "20261022003116" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261022003116_outbound_message_refs.sql"))
	upTo := migrationsWhere(t, func(name string) bool { return name < "20261022014358" })
	_, err = db.MigrateFS(t.Context(), sqlDB, upTo)
	require.NoError(t, err)
	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// An Instance whose rows name Contacts that are erased (before the references were foreign keys)
// migrates: the keys are added NOT VALID, so the old rows are left alone.
func TestMigrateAddsTheContactKeysNotValidOverDanglingIds(t *testing.T) {
	sqlDB := openEmpty(t)
	client := migrateWithDanglingContactRefs(t, sqlDB)

	assert.Equal(t, []string{
		"automation_runs_contacts_contact", "broadcast_recipients_contacts_contact", "confirmations_contacts_contact",
		"outbound_messages_contacts_contact", "suppressions_contacts_contact", "unsubscribes_contacts_contact",
	}, unvalidatedContactKeys(t, sqlDB))
	err := client.OutboundMessage.UpdateOneID(2).SetStatus(outboundmessage.StatusFailed).Exec(t.Context())
	require.NoError(t, err, "a message naming an erased Contact can still change")
}

// The job settles exactly the rows that name a Contact that is gone, the way the key would have
// (a Confirmation dies with it, an opt-out and a message only lose the id), keeps the rows that
// resolve, then validates the keys; once it has run it finds nothing.
func TestClearContactRefsSettlesDanglingRowsThenValidatesTheKeys(t *testing.T) {
	sqlDB := openEmpty(t)
	client := migrateWithDanglingContactRefs(t, sqlDB)

	// A batch of one makes the job loop.
	settled, err := refsettle.New(client).ClearContactRefs(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, settled)

	confirmed, err := client.Confirmation.Query().Order(ent.Asc(confirmation.FieldID)).All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"here@x.test"}, lo.Map(confirmed, func(c *ent.Confirmation, _ int) string { return c.Destination }))

	optOuts, err := client.Unsubscribe.Query().Order(ent.Asc(unsubscribe.FieldID)).All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*int64{new(int64(1)), nil}, lo.Map(optOuts, func(u *ent.Unsubscribe, _ int) *int64 { return u.ContactID }))
	assert.Equal(t, "gone@x.test", optOuts[1].Destination, "the refusal stays")

	messages, err := client.OutboundMessage.Query().Order(ent.Asc(outboundmessage.FieldID)).All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*int64{new(int64(1)), nil}, lo.Map(messages, func(m *ent.OutboundMessage, _ int) *int64 { return m.ContactID }))

	assert.Empty(t, unvalidatedContactKeys(t, sqlDB))

	settled, err = refsettle.New(client).ClearContactRefs(t.Context(), 1)
	require.NoError(t, err)
	assert.Zero(t, settled)
}

func migrateWithDanglingContactRefs(t *testing.T, sqlDB *sql.DB) *ent.Client {
	t.Helper()
	before := migrationsWhere(t, func(name string) bool { return name < "20261022014358" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261022014358_contact_refs.sql"))
	all := migrationsWhere(t, func(string) bool { return true })
	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)
	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// unvalidatedContactKeys is the Contact keys still NOT VALID, over every table that references one.
func unvalidatedContactKeys(t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()
	tables := []string{"automation_runs", "broadcast_recipients", "confirmations", "outbound_messages", "suppressions", "unsubscribes"}
	return lo.FlatMap(tables, func(table string, _ int) []string {
		return lo.Filter(testhelper.UnvalidatedConstraints(t, sqlDB, table), func(name string, _ int) bool {
			return strings.HasSuffix(name, "_contacts_contact")
		})
	})
}

// An Instance with no Enrollment to move ends with the column the ent schema declares: NOT NULL.
func TestMigrateMakesTheEnrollmentVersionRequiredWhenNothingNeedsBackfilling(t *testing.T) {
	sqlDB := openEmpty(t)
	all := migrationsWhere(t, func(string) bool { return true })
	_, err := db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)
	nullable := testhelper.ColumnIsNullable(t, sqlDB, "automation_runs", "version_id")
	assert.False(t, nullable)
}

// The backfill job gives every Automation made before versions its version 1, holding the steps it
// had, and points its Enrollments at it; only then does the migration drop the column.
func TestBackfillGivesEveryAutomationVersionOneAndMovesItsEnrollments(t *testing.T) {
	sqlDB := openEmpty(t)
	legacy := migrationsWhere(t, func(name string) bool { return name < "20261020210001" })
	_, err := db.MigrateFS(t.Context(), sqlDB, legacy)
	require.NoError(t, err)
	testhelper.LoadLegacyFixtures(t, sqlDB, legacyFixtures("20261020210003_drop_automation_definition.sql"))
	all := migrationsWhere(t, func(string) bool { return true })
	beforeDrop := migrationsWhere(t, func(name string) bool { return name < "20261020210003" })
	_, err = db.MigrateFS(t.Context(), sqlDB, beforeDrop)
	require.NoError(t, err)
	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })

	// A batch of one makes the job loop, and a second run finds nothing left.
	made, pointed, err := jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, made)
	assert.Equal(t, 3, pointed)
	made, pointed, err = jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Zero(t, made+pointed)
	assert.False(t, testhelper.ColumnIsNullable(t, sqlDB, "automation_runs", "version_id"), "the finished backfill makes the column NOT NULL")

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err, "the guard lets the drop through once the data has moved")
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "automations", "definition"))

	versions, err := client.AutomationVersion.Query().All(t.Context())
	require.NoError(t, err)
	steps := map[int64]string{}
	for _, v := range versions {
		assert.Equal(t, 1, v.Version)
		steps[v.AutomationID] = v.Steps
	}
	assert.Equal(t, map[int64]string{1: `[{"type":"wait","seconds":1}]`, 2: `[{"type":"wait","seconds":2}]`}, steps)
	runs, err := client.AutomationRun.Query().All(t.Context())
	require.NoError(t, err)
	require.Len(t, runs, 3)
	versionOf := map[int64]int64{}
	for _, v := range versions {
		versionOf[v.AutomationID] = v.ID
	}
	for _, r := range runs {
		assert.Equal(t, versionOf[r.AutomationID], r.VersionID, "run %d", r.ID)
	}

	// After the drop the job still plans (the column is read as JSON) and finds nothing to do.
	made, pointed, err = jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Zero(t, made+pointed)
}
