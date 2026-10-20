package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	acmeJoinRecord = "_sphericon-join.acme.com"
	acmeJoinToken  = "sphericon-join=acme-verified-token"
)

func joinModule(env *testhelper.TestEnv, zone *dnstest.Zone) *joindomains.Module {
	return joindomains.New(env.DB, env.Bus, zone.LookupTXT, time.Now, jobs.OwnerNotifier(env.DB, env.SystemMail))
}

func TestVerifyJoinDomainWorkerKeepsADomainWhoseRecordIsStillPublished(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zone := dnstest.NewZone(t)
	zone.PublishTXT(t, acmeJoinRecord, acmeJoinToken)
	members := env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx)

	err := jobs.NewVerifyJoinDomainWorker(joinModule(env, zone)).
		Work(ctx, job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainVerifiedID}))

	require.NoError(t, err)
	row := env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainVerifiedID)
	assert.NotNil(t, row.VerifiedAt, "a published record keeps the domain verified")
	assert.WithinDuration(t, time.Now(), *row.LastCheckedAt, time.Minute)
	assert.Empty(t, env.SystemMail.Messages())
	assert.Equal(t, members, env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx))
}

func TestVerifyJoinDomainWorkerRevertsADomainWhoseRecordIsGoneAndMailsTheOwnersOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zone := dnstest.NewZone(t)
	w := jobs.NewVerifyJoinDomainWorker(joinModule(env, zone))
	members := env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx)
	args := job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainVerifiedID})

	require.NoError(t, w.Work(ctx, args))
	require.NoError(t, w.Work(ctx, args), "the next tick finds it already unverified")

	row := env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainVerifiedID)
	assert.Nil(t, row.VerifiedAt)
	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "one mail per loss")
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Equal(t, "Action required: join domain acme.com is no longer verified", msgs[0].Subject)
	assert.Contains(t, msgs[0].Text, acmeJoinRecord)
	assert.Equal(t, members, env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx), "Memberships stay")
}

func TestVerifyJoinDomainWorkerRevertsADomainWhoseRecordCarriesAnotherValue(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zone := dnstest.NewZone(t)
	zone.PublishTXT(t, acmeJoinRecord, "sphericon-join=someone-else")

	err := jobs.NewVerifyJoinDomainWorker(joinModule(env, zone)).
		Work(ctx, job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainVerifiedID}))

	require.NoError(t, err)
	assert.Nil(t, env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainVerifiedID).VerifiedAt)
}

func TestVerifyJoinDomainWorkerKeepsStateOnAResolverFailureAndRetries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	failing := dnstest.Resolver(t, map[string]mockdns.Zone{acmeJoinRecord + ".": {Err: errors.New("servfail")}})
	mod := joindomains.New(env.DB, env.Bus, failing.LookupTXT, time.Now, jobs.OwnerNotifier(env.DB, env.SystemMail))

	err := jobs.NewVerifyJoinDomainWorker(mod).
		Work(ctx, job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainVerifiedID}))

	require.Error(t, err, "a failing resolver fails the job so river retries it")
	assert.NotNil(t, env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainVerifiedID).VerifiedAt)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestVerifyJoinDomainWorkerLeavesAPendingDomainAlone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := jobs.NewVerifyJoinDomainWorker(joinModule(env, dnstest.NewZone(t))).
		Work(ctx, job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainPendingID}))

	require.NoError(t, err)
	assert.Nil(t, env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainPendingID).LastCheckedAt, "pending domains are not re-checked")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestVerifyJoinDomainWorkerSurvivesAFailingMail(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.SystemMail.SetErr(errors.New("smtp down"))

	err := jobs.NewVerifyJoinDomainWorker(joinModule(env, dnstest.NewZone(t))).
		Work(ctx, job(jobs.VerifyJoinDomainArgs{JoinDomainID: fixtures.JoinDomainVerifiedID}))

	require.NoError(t, err)
	assert.Nil(t, env.DB.JoinDomain.GetX(ctx, fixtures.JoinDomainVerifiedID).VerifiedAt, "the flip is stored")
}

func TestRecheckJoinDomainsWorkerQueuesOnlyVerifiedDomains(t *testing.T) {
	e := newRiverEnv(t)
	ctx := context.Background()
	verified := e.DB.JoinDomain.Query().Where(joindomain.VerifiedAtNotNil()).CountX(ctx)
	require.Positive(t, verified)

	err := jobs.NewRecheckJoinDomainsWorker(joinModule(e.TestEnv, dnstest.NewZone(t))).
		Work(e.workCtx(), job(jobs.RecheckJoinDomainsArgs{}))

	require.NoError(t, err)
	kinds := e.queued(t)
	assert.Len(t, kinds, verified)
	for _, k := range kinds {
		assert.Equal(t, "join_domain_verify", k)
	}
}

func TestJoinDomainsDueAreTheVerifiedOnes(t *testing.T) {
	env := testhelper.Setup(t)
	mod := joinModule(env, dnstest.NewZone(t))

	ids, err := mod.Due(context.Background(), 100)

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.JoinDomainVerifiedID}, ids, "the pending domains are not due")
}
