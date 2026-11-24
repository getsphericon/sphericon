package engagementtracking_test

import (
	"testing"

	"entgo.io/ent/dialect/sql"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The three Contacts the SQL form is read over: Alice refused tracking, Bob granted it, Liam has
// nothing recorded.
var trio = []int64{fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactLiamID}

// recordTrio records Alice's refusal and Bob's grant, as a member does on the Contact card.
func recordTrio(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	m := testhelper.Module[*engagementtracking.Module](env)
	s := env.DB.Scoped(fixtures.AcmeID)
	for address, state := range map[string]engagementtracking.State{fixtures.ContactAliceEmail: engagementtracking.Denied, fixtures.ContactBobEmail: engagementtracking.Granted} {
		_, err := m.Record(t.Context(), s, engagementtracking.RecordInput{Channel: channels.Email, Destination: address, State: state, Provenance: engagementtracking.Manual})
		require.NoError(t, err)
	}
}

// trioWhere is the Contacts of the trio the predicate keeps.
func trioWhere(t *testing.T, env *testhelper.TestEnv, where func(channels.Channel, engagementtracking.Decision, string) func(*sql.Selector), d engagementtracking.Decision) []int64 {
	t.Helper()
	ids, err := env.DB.Scoped(fixtures.AcmeID).Contact().Query().
		Where(contact.IDIn(trio...), predicate.Contact(where(channels.Email, d, contact.FieldEmail))).
		IDs(t.Context())
	require.NoError(t, err)
	return ids
}

// Under track, every Destination is tracked but one that refused; the complement is that one.
func TestTheSQLFormTracksAllButARefusalUnderTrack(t *testing.T) {
	env := testhelper.Setup(t)
	recordTrio(t, env)

	tracked := trioWhere(t, env, engagementtracking.TrackedWhere, engagementtracking.Track)
	untracked := trioWhere(t, env, engagementtracking.UntrackedWhere, engagementtracking.Track)

	assert.ElementsMatch(t, []int64{fixtures.ContactBobID, fixtures.ContactLiamID}, tracked)
	assert.ElementsMatch(t, []int64{fixtures.ContactAliceID}, untracked)
}

// Under dont_track, only a Destination that granted is tracked.
func TestTheSQLFormTracksOnlyAGrantUnderDontTrack(t *testing.T) {
	env := testhelper.Setup(t)
	recordTrio(t, env)

	tracked := trioWhere(t, env, engagementtracking.TrackedWhere, engagementtracking.DontTrack)
	untracked := trioWhere(t, env, engagementtracking.UntrackedWhere, engagementtracking.DontTrack)

	assert.ElementsMatch(t, []int64{fixtures.ContactBobID}, tracked)
	assert.ElementsMatch(t, []int64{fixtures.ContactAliceID, fixtures.ContactLiamID}, untracked)
}

// One Destination's effective consent follows the Workspace's default where nothing is recorded,
// and the consent where one is.
func TestTheEffectiveConsentOfOneDestinationFollowsTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	recordTrio(t, env)
	m := testhelper.Module[*engagementtracking.Module](env)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := testhelper.Module[*workspaces.Module](env).SetEngagementTrackingDefault(t.Context(), s,
		env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID)), workspace.EngagementTrackingDefaultDontTrack)
	require.NoError(t, err)

	liam, err := m.Of(t.Context(), s, channels.Email, fixtures.ContactLiamEmail)
	require.NoError(t, err)
	bob, err := m.Of(t.Context(), s, channels.Email, fixtures.ContactBobEmail)
	require.NoError(t, err)

	assert.Equal(t, engagementtracking.DontTrack, liam.Effective, "nothing recorded: the default")
	assert.Equal(t, engagementtracking.Track, bob.Effective, "granted")
}
