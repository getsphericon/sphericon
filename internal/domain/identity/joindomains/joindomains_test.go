package joindomains_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(t *testing.T) (*testhelper.TestEnv, *joindomains.Module, *dnstest.Zone) {
	t.Helper()
	zone := dnstest.NewZone(t)
	env := testhelper.Setup(t, testhelper.WithTXTLookup(zone.LookupTXT))
	return env, joindomains.New(env.DB, env.Bus, zone.LookupTXT, time.Now), zone
}

func acting(env *testhelper.TestEnv, workspaceID int64) *ent.Scoped {
	return env.Bus.Act(env.DB.Scoped(workspaceID), events.Actor{Kind: events.ActorUser, ID: "1", Name: "Owner"})
}

// A domain the Workspace already has is refused by joindomain_workspace_id_domain, and the
// Workspace keeps its one claim.
func TestAddRefusesADomainTheWorkspaceHas(t *testing.T) {
	env, m, _ := setup(t)
	s := acting(env, fixtures.AcmeID)

	_, err := m.Add(t.Context(), s, fixtures.JoinDomainPendingDomain)

	require.ErrorIs(t, err, joindomains.ErrAlreadyExists)
	n, err := s.JoinDomain().Query().Where(joindomain.Domain(fixtures.JoinDomainPendingDomain)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

// Another Workspace that verifies the domain between the lookup of its holder and the write wins:
// joindomain_domain refuses the late one with ErrHeldElsewhere, and its claim stays pending.
func TestVerifyLosesToAWorkspaceThatVerifiedInBetween(t *testing.T) {
	env, m, zone := setup(t)
	claim, err := env.DB.JoinDomain.Get(t.Context(), fixtures.JoinDomainGlobexHeldClaimID)
	require.NoError(t, err)
	record := joindomains.RecordOf(claim)
	// Only Globex publishes: Acme's hold is stale, so the domain moves to Globex.
	zone.PublishTXT(t, record.Host, record.Value)
	env.Bus.Use(hook.If(heldAgainJustBefore, globexVerifies))
	s := acting(env, fixtures.GlobexID)

	_, err = m.Verify(t.Context(), s, fixtures.JoinDomainGlobexHeldClaimID)

	require.ErrorIs(t, err, joindomains.ErrHeldElsewhere)
	after, err := s.JoinDomain().Get(t.Context(), fixtures.JoinDomainGlobexHeldClaimID)
	require.NoError(t, err)
	assert.False(t, joindomains.Verified(after), "the late claim stays pending")
}

// globexVerifies is the write that verifies Globex's claim of held.test.
func globexVerifies(_ context.Context, m ent.Mutation) bool {
	jm, ok := m.(*ent.JoinDomainMutation)
	if !ok {
		return false
	}
	id, exists := jm.ID()
	_, verifies := jm.VerifiedAt()
	return exists && verifies && id == fixtures.JoinDomainGlobexHeldClaimID
}

// heldAgainJustBefore is an ent hook on Globex's verifying write: Acme verifies held.test again
// first, as a concurrent check that found its record would.
func heldAgainJustBefore(next ent.Mutator) ent.Mutator {
	return hook.JoinDomainFunc(func(ctx context.Context, m *ent.JoinDomainMutation) (ent.Value, error) {
		if err := m.Client().JoinDomain.UpdateOneID(fixtures.JoinDomainAcmeHeldID).SetVerifiedAt(time.Now()).Exec(ctx); err != nil {
			return nil, err
		}
		return next.Mutate(ctx, m)
	})
}
