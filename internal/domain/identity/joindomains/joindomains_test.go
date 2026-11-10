package joindomains_test

import (
	"context"
	"net"
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
	"github.com/getsphericon/sphericon/internal/infra/devdns"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
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

// A Recheck that read Acme's verification of held.test and found its record gone clears that
// verification only: while its lookup ran, the domain went to Globex and came back verified again,
// and the stale Recheck leaves the new verification in place.
func TestRecheckLeavesAVerificationMadeSinceItsRead(t *testing.T) {
	env, _, zone := setup(t)
	acme, globex := heldClaims(t, env)
	var m *joindomains.Module
	lookup := goneOnLookup(zone, joindomains.RecordOf(acme).Host, 1, func(ctx context.Context) error {
		return handedOverAndBack(ctx, t, env, zone, m, acme, globex)
	})
	m = joindomains.New(env.DB, env.Bus, lookup, time.Now)

	flipped, err := m.Recheck(t.Context(), acme.ID)

	require.NoError(t, err)
	assert.False(t, flipped, "the stale Recheck reverts nothing")
	after := reread(t, env, acme)
	require.True(t, joindomains.Verified(after), "Acme's new verification stays")
	assert.True(t, after.VerifiedAt.After(*acme.VerifiedAt), "it is the verification made since the read")
}

// A hand-over clears only the holder's verification whose record it found gone: when Acme lost
// held.test and verified it again while Globex's Verify looked its record up, Acme keeps the new
// verification and Globex's claim is ErrHeldElsewhere.
func TestVerifyLeavesAHolderVerifiedAgainSinceItsRead(t *testing.T) {
	env, _, zone := setup(t)
	acme, globex := heldClaims(t, env)
	record := joindomains.RecordOf(globex)
	zone.PublishTXT(t, record.Host, record.Value)
	var m *joindomains.Module
	// Both claims share the record's host: Globex's Verify looks it up for its own token, then for
	// the holder's.
	lookup := goneOnLookup(zone, record.Host, 2, func(ctx context.Context) error {
		return revertedAndVerifiedAgain(ctx, t, env, zone, m, acme)
	})
	m = joindomains.New(env.DB, env.Bus, lookup, time.Now)

	_, err := m.Verify(t.Context(), acting(env, fixtures.GlobexID), globex.ID)

	require.ErrorIs(t, err, joindomains.ErrHeldElsewhere)
	after := reread(t, env, acme)
	require.True(t, joindomains.Verified(after), "Acme's new verification stays")
	assert.True(t, after.VerifiedAt.After(*acme.VerifiedAt), "it is the verification made since the read")
}

// The end-to-end suite's resolver answers a Join domain's proof record from the stored token, so a
// domain an Owner adds verifies in dev and e2e without DNS.
func TestDevRecordsAnswersAJoinDomainsRecordFromItsToken(t *testing.T) {
	env := testhelper.Setup(t)
	m := joindomains.New(env.DB, env.Bus, devdns.Lookup(joindomains.DevRecords(env.DB)), time.Now)
	s := acting(env, fixtures.AcmeID)

	row, err := m.Verify(t.Context(), s, fixtures.JoinDomainPendingID)

	require.NoError(t, err)
	assert.True(t, joindomains.Verified(row))
}

// A domain nobody claims has no record, and a name that is no Join domain's record is not its own.
func TestDevRecordsHasNoneForAnUnclaimedDomainOrAnotherName(t *testing.T) {
	env := testhelper.Setup(t)
	records := joindomains.DevRecords(env.DB)

	for _, name := range []string{"_sphericon-join.unclaimed.test.", "_dmarc." + fixtures.JoinDomainPendingDomain + "."} {
		got, err := records(t.Context(), name)

		require.NoError(t, err)
		assert.Empty(t, got, name)
	}
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

// heldClaims are the two claims of held.test as the fixtures hold them: Acme's, verified, and
// Globex's, pending. Neither record is published.
func heldClaims(t *testing.T, env *testhelper.TestEnv) (acme, globex *ent.JoinDomain) {
	t.Helper()
	acme, err := env.DB.JoinDomain.Get(t.Context(), fixtures.JoinDomainAcmeHeldID)
	require.NoError(t, err)
	globex, err = env.DB.JoinDomain.Get(t.Context(), fixtures.JoinDomainGlobexHeldClaimID)
	require.NoError(t, err)
	return acme, globex
}

// reread is the claim's row as it is stored now.
func reread(t *testing.T, env *testhelper.TestEnv, claim *ent.JoinDomain) *ent.JoinDomain {
	t.Helper()
	row, err := env.DB.JoinDomain.Get(t.Context(), claim.ID)
	require.NoError(t, err)
	return row
}

// goneOnLookup resolves through zone, but its nth lookup of host runs meanwhile first and then
// answers the record gone, as it was when that lookup was asked: a lookup that raced the writes of
// meanwhile.
func goneOnLookup(zone *dnstest.Zone, host string, nth int, meanwhile func(context.Context) error) domainname.TXTLookup {
	lookups := 0
	return func(ctx context.Context, name string) ([]string, error) {
		if name != host+"." {
			return zone.LookupTXT(ctx, name)
		}
		lookups++
		if lookups != nth {
			return zone.LookupTXT(ctx, name)
		}
		if err := meanwhile(ctx); err != nil {
			return nil, err
		}
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
}

// handedOverAndBack moves held.test from Acme to Globex and back through Verify: Globex publishes
// and takes it over, then Globex withdraws and Acme publishes and takes it back, verified anew.
func handedOverAndBack(ctx context.Context, t *testing.T, env *testhelper.TestEnv, zone *dnstest.Zone, m *joindomains.Module, acme, globex *ent.JoinDomain) error {
	t.Helper()
	globexRecord, acmeRecord := joindomains.RecordOf(globex), joindomains.RecordOf(acme)
	zone.PublishTXT(t, globexRecord.Host, globexRecord.Value)
	if _, err := m.Verify(ctx, acting(env, fixtures.GlobexID), globex.ID); err != nil {
		return err
	}
	zone.WithdrawTXT(globexRecord.Host)
	zone.PublishTXT(t, acmeRecord.Host, acmeRecord.Value)
	_, err := m.Verify(ctx, acting(env, fixtures.AcmeID), acme.ID)
	return err
}

// revertedAndVerifiedAgain has a re-check revert Acme's held.test, whose record is not published,
// then Acme publish it and verify the domain anew.
func revertedAndVerifiedAgain(ctx context.Context, t *testing.T, env *testhelper.TestEnv, zone *dnstest.Zone, m *joindomains.Module, acme *ent.JoinDomain) error {
	t.Helper()
	if _, err := m.Recheck(ctx, acme.ID); err != nil {
		return err
	}
	record := joindomains.RecordOf(acme)
	zone.PublishTXT(t, record.Host, record.Value)
	_, err := m.Verify(ctx, acting(env, fixtures.AcmeID), acme.ID)
	return err
}
