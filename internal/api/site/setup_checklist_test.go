package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	todo    = siteapi.SiteSetupStepStatusTodo
	pending = siteapi.SiteSetupStepStatusPending
	done    = siteapi.SiteSetupStepStatusDone
)

func setupChecklist(t *testing.T, email string, workspaceID siteapi.WorkspaceId) *siteapi.SiteSetupProgress {
	t.Helper()
	c := testhelper.Setup(t).SiteActor(t, email)
	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	out, ok := res.(*siteapi.SiteSetupProgress)
	require.Truef(t, ok, "got %T", res)
	return out
}

func TestSiteSetupChecklistRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteSetupChecklistHidesAForeignWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSetupChecklistGetNotFound{}, res)
}

func TestSiteSetupChecklistStates(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		workspaceID siteapi.WorkspaceId
		want        siteapi.SiteSetupSteps
		complete    bool
	}{
		{"a Workspace with nothing is all todo; a skipped message is not a sent email", fixtures.OwnerSallyEmail, fixtures.HooliPublicID,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: todo, Contact: todo, SentEmail: todo}, false},
		{"a domain waiting for DNS is pending and blocks nothing", fixtures.OwnerJaneEmail, fixtures.GlobexPublicID,
			siteapi.SiteSetupSteps{Integration: done, SendingDomain: pending, Contact: done, SentEmail: done}, false},
		{"a verified domain, Contacts and Broadcasts without an Integration", fixtures.InitechOwnerOlgaEmail, fixtures.InitechPublicID,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: done, Contact: done, SentEmail: done}, false},
		{"a Transactional-only Workspace completes the last step", fixtures.KramericaOwnerNewmanEmail, fixtures.KramericaPublicID,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: done, Contact: todo, SentEmail: done}, false},
		{"a fully set up Workspace is complete", fixtures.OwnerJohnEmail, fixtures.AcmePublicID,
			siteapi.SiteSetupSteps{Integration: done, SendingDomain: done, Contact: done, SentEmail: done}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := setupChecklist(t, tc.owner, tc.workspaceID)
			assert.Equal(t, tc.want, got.Steps)
			assert.Equal(t, tc.complete, got.CompletedAt.Set, "complete is having a moment")
		})
	}
}

func TestAChecklistFoundCompleteStaysCompleteWhenAStepRegresses(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	read := func() *siteapi.SiteSetupProgress {
		t.Helper()
		res, err := c.SiteSetupChecklistGet(t.Context(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		out, ok := res.(*siteapi.SiteSetupProgress)
		require.Truef(t, ok, "got %T", res)
		return out
	}
	first := read()
	_, err := env.DB.Scoped(fixtures.AcmeID).SendingDomain().Update().
		Where(sendingdomain.VerifiedAtNotNil()).SetVerificationLostAt(time.Now()).Save(t.Context())
	require.NoError(t, err)

	after := read()

	assert.True(t, first.CompletedAt.Set, "the read that finds every step done stamps it")
	assert.Equal(t, pending, after.Steps.SendingDomain, "the steps are still read now")
	assert.Equal(t, first.CompletedAt.Value, after.CompletedAt.Value, "the first moment is kept")
}

func TestAChecklistNeverCompleteHasNoMoment(t *testing.T) {
	got := setupChecklist(t, fixtures.OwnerSallyEmail, fixtures.HooliPublicID)

	assert.False(t, got.CompletedAt.Set)
}
