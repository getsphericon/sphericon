package admission_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/admission"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella requires a Second factor since 2026-03-01, and Rita's grace there ended on 2026-03-08.
var afterRitasGrace = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

func newModule(env *testhelper.TestEnv) *admission.Module {
	acceptance := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	factors := secondfactors.New(env.DB, env.Bus, env.Cipher, acceptance, time.Now)
	return admission.New(memberships.New(env.DB, env.Bus), acceptance, factors)
}

func TestAMemberIsAdmittedToTheirWorkspaceWithItsScopeAndTheirMembership(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := newModule(env).Admit(t.Context(), admission.Request{UserID: fixtures.MemberMaryID, Workspace: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeMemberMembershipID, got.Membership.ID)
	assert.Equal(t, fixtures.AcmePublicID, got.Membership.Edges.Workspace.PublicID, "the Workspace is loaded")
	assert.EqualValues(t, fixtures.MemberMaryID, got.Membership.Edges.User.ID, "and the User")
	require.NotNil(t, got.Scoped)
	assert.EqualValues(t, fixtures.AcmeID, got.Scoped.WorkspaceID())
}

func TestARequestOfNoWorkspaceAdmitsTheUserAlone(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := newModule(env).Admit(t.Context(), admission.Request{UserID: fixtures.MemberMaryID})

	require.NoError(t, err)
	assert.Equal(t, admission.Admitted{}, got)
}

// The refusals come in one order on every surface: a Workspace the User is no member of before a
// required acceptance pending, and that before the Two-factor requirement.
func TestTheRefusalsComeInOneOrder(t *testing.T) {
	env := testhelper.Setup(t)
	module := newModule(env)

	_, notMember := module.Admit(t.Context(), admission.Request{UserID: fixtures.UmbrellaOwnerRitaID, Workspace: fixtures.KramericaPublicID})
	_, withheld := module.Admit(t.Context(), admission.Request{UserID: fixtures.UmbrellaOwnerRitaID, Workspace: fixtures.UmbrellaPublicID, At: afterRitasGrace})
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	_, pendingAndNotMember := module.Admit(t.Context(), admission.Request{UserID: fixtures.UmbrellaOwnerRitaID, Workspace: fixtures.KramericaPublicID})
	_, pendingAndWithheld := module.Admit(t.Context(), admission.Request{UserID: fixtures.UmbrellaOwnerRitaID, Workspace: fixtures.UmbrellaPublicID, At: afterRitasGrace})
	_, letThrough := module.Admit(t.Context(), admission.Request{
		UserID: fixtures.UmbrellaOwnerRitaID, Workspace: fixtures.UmbrellaPublicID, At: afterRitasGrace,
		WhileAcceptancePending: true, StrongSession: true,
	})

	require.ErrorIs(t, notMember, workspaces.ErrNotFound)
	require.ErrorIs(t, withheld, secondfactors.ErrSecondFactorRequired)
	require.ErrorIs(t, pendingAndNotMember, workspaces.ErrNotFound)
	require.ErrorIs(t, pendingAndWithheld, users.ErrAcceptancePending)
	require.NoError(t, letThrough, "what the operation lets through and a strong session")
}

func TestAUserWithARequiredAcceptancePendingIsRefusedInAndOutOfAWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	module := newModule(env)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	_, inWorkspace := module.Admit(t.Context(), admission.Request{UserID: fixtures.MemberMaryID, Workspace: fixtures.AcmePublicID})
	_, outside := module.Admit(t.Context(), admission.Request{UserID: fixtures.MemberMaryID})
	_, letThrough := module.Admit(t.Context(), admission.Request{UserID: fixtures.MemberMaryID, Workspace: fixtures.AcmePublicID, WhileAcceptancePending: true})

	require.ErrorIs(t, inWorkspace, users.ErrAcceptancePending)
	require.ErrorIs(t, outside, users.ErrAcceptancePending)
	require.NoError(t, letThrough)
}

// The Two-factor requirement withholds a member once their grace ends, unless the session is
// strong or the operation lets a withheld User through, and never outside its Workspace.
func TestTheTwoFactorRequirementLetsThroughWhatTheRequestAllows(t *testing.T) {
	env := testhelper.Setup(t)
	module := newModule(env)
	admit := func(r admission.Request) error {
		r.UserID, r.Workspace = fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID
		_, err := module.Admit(t.Context(), r)
		return err
	}

	require.ErrorIs(t, admit(admission.Request{At: afterRitasGrace}), secondfactors.ErrSecondFactorRequired)
	require.NoError(t, admit(admission.Request{At: afterRitasGrace, StrongSession: true}), "a strong session meets the requirement")
	require.NoError(t, admit(admission.Request{At: afterRitasGrace, WhileWithheld: true}), "an operation that lets a withheld User through")
	_, outside := module.Admit(t.Context(), admission.Request{UserID: fixtures.UmbrellaOwnerRitaID, At: afterRitasGrace})
	require.NoError(t, outside, "an operation of no Workspace")
}

// A token of the User's own is admitted to the Membership it names as their session would be:
// the acceptance first, then the Two-factor requirement, and nothing lets it through.
func TestATokenIsAdmittedToItsMembershipAsTheSessionWouldBe(t *testing.T) {
	env := testhelper.Setup(t)
	module := newModule(env)
	_, rita, err := memberships.New(env.DB, env.Bus).Scope(t.Context(), fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)
	require.NoError(t, err)

	withheld := module.AdmitMembership(t.Context(), rita, afterRitasGrace)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	pendingAndWithheld := module.AdmitMembership(t.Context(), rita, afterRitasGrace)

	require.ErrorIs(t, withheld, secondfactors.ErrSecondFactorRequired)
	require.ErrorIs(t, pendingAndWithheld, users.ErrAcceptancePending)
}
