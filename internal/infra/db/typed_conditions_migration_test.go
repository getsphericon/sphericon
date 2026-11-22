package db_test

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// typedConditionsBefore is a database of the schema before the stored Conditions were typed, with
// the rows of testdata/<dir> in the old shape.
func typedConditionsBefore(t *testing.T, dir string) *sql.DB {
	t.Helper()
	return testhelper.SchemaBefore(t, "condition_entries_default", filepath.Join("testdata", dir))
}

// The typed tree each Segment of testdata/typed_conditions holds after the migration, by id.
var typedSegments = map[int64]string{
	1:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"first_name","test":{"kind":"text","operator":"eq","value":"Ann"}}]}`,
	2:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"first_name","test":{"kind":"text","operator":"neq","value":"Ann"}}]}`,
	3:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"contains","value":"AR"}}]}`,
	4:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"beginsWith","value":"b"}}]}`,
	5:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"last_name","test":{"kind":"presence","operator":"notSet"}}]}`,
	6:  `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"last_name","test":{"kind":"presence","operator":"set"}}]}`,
	7:  `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}`,
	8:  `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"doesNotHave"}]}`,
	9:  `{"kind":"group","combinator":"or","entries":[{"kind":"telegram","operator":"has"},{"kind":"sms","operator":"has"}]}`,
	10: `{"kind":"group","combinator":"and","entries":[{"kind":"telegram","operator":"doesNotHave"},{"kind":"sms","operator":"doesNotHave"}]}`,
	11: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`,
	12: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"plan","test":{"kind":"text","operator":"neq","value":"pro"}}]}`,
	13: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"labels","test":{"kind":"text","operator":"contains","value":"alpha"}}]}`,
	14: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"score","test":{"kind":"presence","operator":"set"}}]}`,
	15: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"trial","test":{"kind":"text","operator":"eq","value":"true"}}]}`,
	16: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"score","test":{"kind":"number","operator":"gt","value":5}}]}`,
	17: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"score","test":{"kind":"number","operator":"gte","value":10}}]}`,
	18: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"score","test":{"kind":"number","operator":"lt","value":5}}]}`,
	19: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"score","test":{"kind":"number","operator":"lte","value":2.5}}]}`,
	20: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"renews_at","test":{"kind":"moment","operator":"before","at":"2026-01-01T00:00:00Z"}}]}`,
	21: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"renews_at","test":{"kind":"moment","operator":"after","at":"2026-01-01T03:00:00+03:00"}}]}`,
	22: `{"kind":"group","combinator":"and","entries":[{"kind":"birthday","test":{"kind":"moment","operator":"before","at":"1980-01-01T00:00:00Z"}}]}`,
	23: `{"kind":"group","combinator":"and","entries":[{"kind":"birthday","test":{"kind":"moment","operator":"after","at":"1980-01-01T00:00:00Z"}}]}`,
	24: `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"seen_at","test":{"kind":"withinLast","days":7}}]}`,
	25: `{"kind":"group","combinator":"and","entries":[{"kind":"birthday","test":{"kind":"withinLast","days":20000}}]}`,
	26: `{"kind":"group","combinator":"and","entries":[{"kind":"event","action":"purchase","operator":"atLeast","times":2,"withinDays":7}]}`,
	27: `{"kind":"group","combinator":"and","entries":[{"kind":"event","action":"purchase","operator":"atMost","times":1}]}`,
	28: `{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"attribute","attribute":"first_name","test":{"kind":"presence","operator":"set"}},` +
		`{"kind":"group","combinator":"and","not":true,"entries":[{"kind":"tag","name":"vip","operator":"has"},` +
		`{"kind":"customField","key":"score","test":{"kind":"number","operator":"gt","value":5}}]}]}`,
	29: `{"kind":"group","combinator":"and","entries":[]}`,
	30: `{"kind":"group","combinator":"or","entries":[{"kind":"event","action":"purchase","operator":"atLeast","times":3},` +
		`{"kind":"customField","key":"score","test":{"kind":"number","operator":"lt","value":3}}]}`,
}

// The typed tree each Rule version of testdata/typed_conditions holds after the migration, by id.
var typedRuleVersions = map[int64]string{
	1: `{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"ref","test":{"kind":"presence","operator":"notSet"}},` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"presence","operator":"set"}},` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"beginsWith","value":"https://acme"}},` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"eq","value":"https://acme.test/pricing"}},` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"neq","value":"https://acme.test/"}}]}`,
	2: `{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"contains","value":"PRICING"}},` +
		`{"kind":"tag","name":"vip","operator":"has"}]}`,
	3: `{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"property","of":{"kind":"trigger"},"key":"score","test":{"kind":"number","operator":"gt","value":5}},` +
		`{"kind":"customField","key":"plan","test":{"kind":"text","operator":"eq","value":"free"}}]}`,
	4: `{"kind":"group","combinator":"and","entries":[{"kind":"property","of":{"kind":"trigger"},"key":"score","test":{"kind":"number","operator":"lte","value":5}}]}`,
}

// The typed tree each armed Broadcast's copy holds after the migration, by id.
var typedBroadcasts = map[int64]string{
	1: `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"},` +
		`{"kind":"customField","key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`,
	2: `{"kind":"group","combinator":"and","entries":[]}`,
}

// recordedEdit is the diff of the Audit entry of testdata/typed_conditions, as it was recorded.
const recordedEdit = `{"condition":{"from":{"kind":"group","combinator":"and","rules":[]},` +
	`"to":{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}}}`

// The upgrade rewrites every stored Condition to the typed tree of its place (ADR 0181): every
// Segment (an archived one too), every Rule version (past, live and draft) and every armed
// Broadcast's copy, each leaf kind and operator to the leaf and typed test it names, a nested
// negated group keeping its negation. A Condition already in the typed shape, as a run interrupted
// after its rewrite leaves it, is left as it is, a Broadcast to all Contacts keeps no Condition, an
// Audit entry keeps the Condition of its diff as it was recorded, and the schema moves to the next
// Major.
func TestTheTypedConditionsMigrationRewritesEveryStoredConditionToItsTypedTree(t *testing.T) {
	sqlDB := typedConditionsBefore(t, "typed_conditions")

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	ctx := t.Context()

	for id, want := range typedSegments {
		got := client.Segment.GetX(ctx, id).Condition
		assert.JSONEq(t, want, testhelper.Document(t, got), "segment %d", id)
	}
	for id, want := range typedRuleVersions {
		got := client.RuleVersion.GetX(ctx, id).Condition
		assert.JSONEq(t, want, testhelper.Document(t, got), "rule version %d", id)
	}
	for id, want := range typedBroadcasts {
		got := client.Broadcast.GetX(ctx, id).Condition
		assert.JSONEq(t, want, testhelper.Document(t, got), "broadcast %d", id)
	}
	toAll := client.Broadcast.GetX(ctx, 3).Condition
	assert.Nil(t, toAll)
	recorded := client.AuditEntry.GetX(ctx, 1).Diff
	assert.JSONEq(t, recordedEdit, testhelper.Document(t, recorded))
	state, err := db.ReadState(ctx, sqlDB)
	require.NoError(t, err)
	assert.Equal(t, db.SchemaState(db.Majors[len(db.Majors)-1]), state)
	assert.Equal(t, 3, state.Major, "the typed Conditions start Major 3")
}

// The Contacts of testdata/typed_conditions.
const (
	ann  = "ann@acme.test"
	bob  = "bob@acme.test"
	cara = "cara@acme.test"
	dan  = "dan@acme.test"
)

// The Contacts each Segment's Condition matched before the migration, by id.
var segmentMatches = map[int64][]string{
	1: {ann}, 2: {bob, cara}, 3: {cara}, 4: {bob}, 5: {ann, cara, dan}, 6: {bob},
	7: {ann, cara}, 8: {bob, dan}, 9: nil, 10: {ann, bob, cara, dan},
	11: {ann, cara}, 12: {bob}, 13: {ann}, 14: {ann, bob}, 15: {ann},
	16: {ann}, 17: {ann}, 18: {bob}, 19: {bob},
	20: {bob}, 21: {ann}, 22: {cara}, 23: {ann}, 24: {ann}, 25: {ann},
	26: {ann}, 27: {bob, cara, dan}, 28: {bob, cara}, 29: {ann, bob, cara, dan}, 30: {ann, bob},
}

// The Contacts each armed Broadcast's copy matched before the migration, by id.
var broadcastMatches = map[int64][]string{1: {ann, cara}, 2: {ann, bob, cara, dan}}

// triggered is the properties of the Event a Rule version's Condition is evaluated for.
var triggered = map[string]any{"url": "https://acme.test/pricing", "score": 7.0, "ref": nil}

// The Contacts each Rule version's Condition matched before the migration for that Event, by id.
var ruleVersionMatches = map[int64][]string{1: {ann, bob, cara, dan}, 2: {ann, cara}, 3: {bob}, 4: nil}

// Each migrated Condition matches, through the Condition module, the Contacts the stored one
// matched before the migration: no test is dropped or changes its meaning.
func TestEveryMigratedConditionMatchesTheContactsItMatchedBefore(t *testing.T) {
	sqlDB := typedConditionsBefore(t, "typed_conditions")
	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	ctx := t.Context()
	s := client.Scoped(1)
	conditions := condition.New(time.Now)

	for id, want := range segmentMatches {
		p, err := conditions.Compile(ctx, s, client.Segment.GetX(ctx, id).Condition)
		require.NoError(t, err, "segment %d", id)
		got := emailsMatching(t, s, p)
		assert.Equal(t, want, got, "segment %d", id)
	}
	for id, want := range broadcastMatches {
		p, err := conditions.Compile(ctx, s, *client.Broadcast.GetX(ctx, id).Condition)
		require.NoError(t, err, "broadcast %d", id)
		got := emailsMatching(t, s, p)
		assert.Equal(t, want, got, "broadcast %d", id)
	}
	for id, want := range ruleVersionMatches {
		p, err := conditions.RulePredicate(ctx, s, client.RuleVersion.GetX(ctx, id).Condition, triggered)
		require.NoError(t, err, "rule version %d", id)
		got := emailsMatching(t, s, p)
		assert.Equal(t, want, got, "rule version %d", id)
	}
}

// emailsMatching is the emails of the Workspace's Contacts p matches, in order, nil for none.
func emailsMatching(t *testing.T, s *ent.Scoped, p predicate.Contact) []string {
	t.Helper()
	emails, err := s.Contact().Query().Where(p).Order(contact.ByEmail()).Select(contact.FieldEmail).Strings(t.Context())
	require.NoError(t, err)
	return lo.Ternary(len(emails) == 0, nil, emails)
}

// unmappable is a stored Condition the migration names as one it cannot rewrite.
type unmappable struct {
	table           string
	id, workspaceID int64
}

// A stored tree the migration cannot map (a field the typed tree has no leaf for, an operator its
// value cannot take, a test of the triggering Event in a Segment's copy, a date leaf without its
// moment, a tree typed at some depths and old at others) stops it with one error
// naming every such row, and nothing is rewritten: a Condition that maps keeps its old shape and the
// schema stays at its Major, so the Self-hoster can fix the rows on the running release and retry.
func TestTheTypedConditionsMigrationNamesEveryConditionItCannotMapAndRewritesNothing(t *testing.T) {
	sqlDB := typedConditionsBefore(t, "typed_conditions_unmappable")
	ctx := t.Context()

	_, err := db.Migrate(ctx, sqlDB)

	refused, ok := errors.AsType[*db.UnmappableConditionsError](err)
	require.True(t, ok, "the migration fails naming the rows: %v", err)
	named := lo.Map(refused.Conditions, func(c db.UnmappableCondition, _ int) unmappable {
		return unmappable{table: c.Table, id: c.ID, workspaceID: c.WorkspaceID}
	})
	assert.Equal(t, []unmappable{
		{table: "segments", id: 1, workspaceID: 1},
		{table: "segments", id: 3, workspaceID: 1},
		{table: "segments", id: 4, workspaceID: 1},
		{table: "segments", id: 5, workspaceID: 1},
		{table: "rule_versions", id: 5, workspaceID: 2},
		{table: "broadcasts", id: 3, workspaceID: 1},
	}, named)
	assert.Contains(t, err.Error(), `segments 1 (Workspace 1): entry 0: entry 0: unknown field "nope"`)
	assert.Contains(t, err.Error(), `segments 3 (Workspace 1): the tree mixes typed groups (entries) with old ones (rules)`)
	assert.Contains(t, err.Error(), `segments 4 (Workspace 1): the tree mixes typed groups (entries) with old ones (rules)`)
	assert.Contains(t, err.Error(), `segments 5 (Workspace 1): entry 0: date leaf moment is required`)
	status, err := db.Status(ctx, sqlDB)
	require.NoError(t, err)
	assert.Less(t, status.Current, int64(db.TypedConditionsVersion), "the migration is still to run")
	assert.Equal(t, 2, status.State.Major)
	mappable := entclient.Core.New(sqlDB).Segment.GetX(ctx, 2).Condition
	assert.Nil(t, mappable.Entries, "a Condition that maps is left in its old shape")
}

// Running the rewrite again over what it wrote changes nothing: every Condition is typed at every
// depth, so it is left as it is (ADR 0033: a Go migration is idempotent, so a re-run after an
// interrupted Upgrade finishes it).
func TestTheTypedConditionsRewriteRunAgainChangesNothing(t *testing.T) {
	sqlDB := typedConditionsBefore(t, "typed_conditions")
	ctx := t.Context()
	_, err := db.Migrate(ctx, sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	first := storedTrees(t, client)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, db.RewriteTypedConditions(ctx, tx))
	require.NoError(t, tx.Commit())

	second := storedTrees(t, client)
	assert.Equal(t, first, second)
}

// storedTrees is every stored Condition as its JSON, by table and id.
func storedTrees(t *testing.T, client *ent.Client) map[string]string {
	t.Helper()
	ctx := t.Context()
	out := map[string]string{}
	for _, s := range client.Segment.Query().AllX(ctx) {
		out[fmt.Sprintf("segments %d", s.ID)] = testhelper.Document(t, s.Condition)
	}
	for _, v := range client.RuleVersion.Query().AllX(ctx) {
		out[fmt.Sprintf("rule_versions %d", v.ID)] = testhelper.Document(t, v.Condition)
	}
	for _, b := range client.Broadcast.Query().AllX(ctx) {
		out[fmt.Sprintf("broadcasts %d", b.ID)] = testhelper.Document(t, b.Condition)
	}
	return out
}
