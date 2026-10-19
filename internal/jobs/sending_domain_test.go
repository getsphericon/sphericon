package jobs_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifySendingDomainByID_becomesVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.False(t, dom.Verified)

	// DNS now publishes the matching key.
	ok, flipped, err := jobs.VerifySendingDomainByID(ctx, env.DB, dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.news.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, flipped, "becoming verified is not a flip-to-unverified")

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, reloaded.Verified)
	require.NotNil(t, reloaded.VerifiedAt)
	require.NotNil(t, reloaded.LastCheckedAt)
}

func TestVerifySendingDomainByID_flipsToUnverifiedWhenRecordGone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Record disappeared → NXDOMAIN → verifies false, no error.
	ok, flipped, err := jobs.VerifySendingDomainByID(ctx, env.DB, dnstest.Resolver(t, nil).LookupTXT, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, flipped, "losing verification of a live domain is the notify-worthy flip")

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, reloaded.Verified, "a verified domain must flip back when its DKIM DNS is gone")
}

// On a verified→unverified flip the workspace owner is emailed (ADR 0010 slice 3).
// Workspace 1's owner is user 1 (info@getsphericon.com), per the membership fixtures.
func TestNotifySendingDomainUnverified_emailsOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := jobs.NotifySendingDomainUnverified(ctx, env.DB, env.SystemMail, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Equal(t, "Action required: sending domain mail.acme.com is no longer verified", msgs[0].Subject)
	assert.Contains(t, msgs[0].Text, "sphericon._domainkey.mail.acme.com")
	assert.Contains(t, msgs[0].Text, "is now blocked")
}

func TestNotifySendingDomainUnverified_nilSenderIsNoop(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, jobs.NotifySendingDomainUnverified(context.Background(), env.DB, nil, fixtures.SendingDomainVerifiedID))
}

// DevTXTLookup echoes each domain's own stored key, so a seeded/added domain
// self-verifies in dev without real DNS (the local send-gate escape, ADR 0010).
func TestDevTXTLookup_trustsSeededDomains(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.False(t, dom.Verified)

	ok, _, err := jobs.VerifySendingDomainByID(ctx, env.DB, jobs.DevTXTLookup(env.DB), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, ok, "dev lookup makes a seeded domain verify")
}

func TestDomainsDueForRecheck_neverCheckedFirst(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Fixtures: id 1 has last_checked_at set; id 2 (news.acme.com) is never
	// checked (NULL). NULLS FIRST must surface id 2 ahead of id 1.
	ids, err := jobs.DomainsDueForRecheck(ctx, env.DB, 100)
	require.NoError(t, err)
	require.Contains(t, ids, int64(fixtures.SendingDomainUnverifiedID))
	require.Contains(t, ids, int64(fixtures.SendingDomainVerifiedID))

	posNull := slices.Index(ids, int64(fixtures.SendingDomainUnverifiedID))
	posChecked := slices.Index(ids, int64(fixtures.SendingDomainVerifiedID))
	assert.Less(t, posNull, posChecked, "never-checked domain must be re-checked before an already-checked one")
}

func TestVerifySendingDomainByID_resolverErrorDoesNotChangeState(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, _, err := jobs.VerifySendingDomainByID(ctx, env.DB, dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {Err: errors.New("servfail")}}).LookupTXT, fixtures.SendingDomainVerifiedID)
	require.Error(t, err)

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.True(t, reloaded.Verified, "a transient resolver failure must not flip verified")
}
