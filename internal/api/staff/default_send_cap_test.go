package staff_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The default Send cap (ADR 0067) a Staff member sets on an Instance that has Send caps (the saas
// Mode, which needs the staff License), recorded in the Instance audit log (ADR 0070), and its
// absence on a self-hosted Instance.

func trySetDefaultSendCap(t *testing.T, c *staffapi.Client, perDay int32) (*staffapi.StaffDefaultSendCapChange, error) {
	t.Helper()
	return c.StaffInstanceSetDefaultSendCap(t.Context(), &staffapi.StaffDefaultSendCapInput{PerDay: staffapi.MaxPerDay(perDay)})
}

func tryClearDefaultSendCap(t *testing.T, c *staffapi.Client) (*staffapi.StaffDefaultSendCapChange, error) {
	t.Helper()
	return c.StaffInstanceClearDefaultSendCap(t.Context())
}

// setDefaultSendCap is trySetDefaultSendCap for a change that is expected to be taken.
func setDefaultSendCap(t *testing.T, c *staffapi.Client, perDay int32) *staffapi.StaffDefaultSendCapChange {
	t.Helper()
	change, err := trySetDefaultSendCap(t, c, perDay)
	require.NoError(t, err)
	return change
}

// clearDefaultSendCap is tryClearDefaultSendCap for a change that is expected to be taken.
func clearDefaultSendCap(t *testing.T, c *staffapi.Client) *staffapi.StaffDefaultSendCapChange {
	t.Helper()
	change, err := tryClearDefaultSendCap(t, c)
	require.NoError(t, err)
	return change
}

func auditPage(t *testing.T, c *staffapi.Client, params staffapi.StaffInstanceListAuditEntriesParams) *staffapi.StaffInstanceAuditEntryList {
	t.Helper()
	page, err := c.StaffInstanceListAuditEntries(t.Context(), params)
	require.NoError(t, err)
	return page
}

// fixtureLog is the Instance audit log of the fixtures (fixtures/instance_audit_entries.yml),
// oldest first: a default Send cap set, raised and cleared.
var fixtureLog = []string{
	`instance.default_send_cap_change {"default_send_cap":{"to":100}}`,
	`instance.default_send_cap_change {"default_send_cap":{"from":100,"to":200}}`,
	`instance.default_send_cap_change {"default_send_cap":{"from":200}}`,
}

// diffs are the entries' actions and diffs as JSON, oldest first.
func diffs(t *testing.T, items []staffapi.StaffInstanceAuditEntryResource) []string {
	t.Helper()
	out := make([]string, len(items))
	for i, e := range items {
		b, err := json.Marshal(e.Diff)
		require.NoError(t, err)
		out[len(items)-1-i] = string(e.Action) + " " + string(b)
	}
	return out
}

func TestAStaffMemberSetsChangesAndClearsTheDefaultSendCap(t *testing.T) {
	h := newLicensedHarness(t)
	c := h.console(t)

	before := instanceState(t, c)
	require.True(t, before.DefaultSendCap.IsSet(), "an Instance with Send caps shows the default")
	assert.True(t, before.DefaultSendCap.Value.PerDay.IsNull(), "an Instance starts with none")

	set := setDefaultSendCap(t, c, 500)
	assert.True(t, set.Changed)
	assert.Equal(t, staffapi.NewNilMaxPerDay(500), set.DefaultSendCap.PerDay)

	changed := setDefaultSendCap(t, c, 2000)
	afterChange := instanceState(t, c)
	assert.True(t, changed.Changed)
	assert.Equal(t, staffapi.NewNilMaxPerDay(2000), afterChange.DefaultSendCap.Value.PerDay)

	cleared := clearDefaultSendCap(t, c)
	afterClear := instanceState(t, c)
	assert.True(t, cleared.Changed)
	assert.True(t, cleared.DefaultSendCap.PerDay.IsNull())
	assert.True(t, afterClear.DefaultSendCap.Value.PerDay.IsNull())

	log := auditPage(t, c, staffapi.StaffInstanceListAuditEntriesParams{})
	recorded := diffs(t, log.Items)
	assert.Equal(t, slices.Concat(fixtureLog, []string{
		`instance.default_send_cap_change {"default_send_cap":{"to":500}}`,
		`instance.default_send_cap_change {"default_send_cap":{"from":500,"to":2000}}`,
		`instance.default_send_cap_change {"default_send_cap":{"from":2000}}`,
	}), recorded, "each change records the old and the new value, absent where there is none")
	for _, e := range log.Items {
		assert.Equal(t, staffapi.EntityId(fixtures.StaffEnrolledID), e.Actor.ID, "the signed-in Staff member is the actor")
		assert.Equal(t, staffapi.EmailAddress(fixtures.StaffEnrolledEmail), e.Actor.Email)
	}
	assert.False(t, log.NextCursor.IsSet(), "one page holds them all")
}

func TestSettingTheSameDefaultSendCapAndClearingNoneChangeNothing(t *testing.T) {
	h := newLicensedHarness(t)
	c := h.console(t)

	none := clearDefaultSendCap(t, c)
	assert.False(t, none.Changed, "an Instance with no default stays as it was")

	setDefaultSendCap(t, c, 500)
	same := setDefaultSendCap(t, c, 500)
	assert.False(t, same.Changed)
	assert.Equal(t, staffapi.NewNilMaxPerDay(500), same.DefaultSendCap.PerDay)

	log := auditPage(t, c, staffapi.StaffInstanceListAuditEntriesParams{})
	recorded := diffs(t, log.Items)
	assert.Equal(t, slices.Concat(fixtureLog, []string{
		`instance.default_send_cap_change {"default_send_cap":{"to":500}}`,
	}), recorded, "only the real change is recorded")
}

func TestTheInstanceAuditLogIsReadPageByPageNewestFirst(t *testing.T) {
	h := newLicensedHarness(t)
	c := h.console(t)

	first := auditPage(t, c, staffapi.StaffInstanceListAuditEntriesParams{Limit: staffapi.NewOptInt32(2)})
	require.True(t, first.NextCursor.IsSet())
	last := auditPage(t, c, staffapi.StaffInstanceListAuditEntriesParams{
		Limit: staffapi.NewOptInt32(2), Cursor: staffapi.NewOptString(first.NextCursor.Value),
	})
	assert.False(t, last.NextCursor.IsSet(), "the last page has no cursor")

	recorded := diffs(t, append(first.Items, last.Items...))
	assert.Equal(t, fixtureLog, recorded)

	_, err := c.StaffInstanceListAuditEntries(t.Context(), staffapi.StaffInstanceListAuditEntriesParams{Cursor: staffapi.NewOptString("nope")})
	testhelper.StaffRefusal(t, err, http.StatusBadRequest)
}

// A self-hosted Instance has no Send cap: the default's endpoints are not found whatever the
// License key, and the Instance page has no default to show.
func TestASelfHostedInstanceHasNoDefaultSendCap(t *testing.T) {
	for name, build := range map[string]func(t *testing.T) *harness{
		"without a license": func(t *testing.T) *harness {
			t.Helper()
			return newHarness(t)
		},
		"with every feature": func(t *testing.T) *harness {
			t.Helper()
			return newLicensedHarness(t, testhelper.WithMode(config.ModeSelfHosted))
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := build(t).console(t)

			_, setErr := trySetDefaultSendCap(t, c, 500)
			_, clearErr := tryClearDefaultSendCap(t, c)
			state := instanceState(t, c)
			log := auditPage(t, c, staffapi.StaffInstanceListAuditEntriesParams{})
			testhelper.StaffRefusal(t, setErr, http.StatusNotFound)
			testhelper.StaffRefusal(t, clearErr, http.StatusNotFound)
			assert.False(t, state.DefaultSendCap.IsSet())
			assert.Equal(t, fixtureLog, diffs(t, log.Items), "the refused calls record nothing")
		})
	}
}
