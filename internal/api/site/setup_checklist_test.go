package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	todo    = siteapi.SiteSetupStepStatusTodo
	pending = siteapi.SiteSetupStepStatusPending
	done    = siteapi.SiteSetupStepStatusDone
)

func setupChecklist(t *testing.T, email, slug string) *siteapi.SiteSetupProgress {
	t.Helper()
	c := testhelper.Setup(t).SiteActor(t, email)
	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{Slug: slug})
	require.NoError(t, err)
	out, ok := res.(*siteapi.SiteSetupProgress)
	require.Truef(t, ok, "got %T", res)
	return out
}

func TestSiteSetupChecklistRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{Slug: fixtures.AcmeSlug})
	require.Error(t, err)
}

func TestSiteSetupChecklistHidesAForeignWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{Slug: fixtures.GlobexSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func TestSiteSetupChecklistStates(t *testing.T) {
	tests := []struct {
		name     string
		owner    string
		slug     string
		want     siteapi.SiteSetupSteps
		complete bool
	}{
		{"a Workspace with nothing is all todo; a skipped message is not a sent email", fixtures.OwnerSallyEmail, fixtures.HooliSlug,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: todo, Contact: todo, SentEmail: todo}, false},
		{"a domain waiting for DNS is pending and blocks nothing", fixtures.OwnerJaneEmail, fixtures.GlobexSlug,
			siteapi.SiteSetupSteps{Integration: done, SendingDomain: pending, Contact: done, SentEmail: done}, false},
		{"a verified domain, Contacts and Broadcasts without an Integration", fixtures.InitechOwnerOlgaEmail, fixtures.InitechSlug,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: done, Contact: done, SentEmail: done}, false},
		{"a Transactional-only Workspace completes the last step", fixtures.KramericaOwnerNewmanEmail, fixtures.KramericaSlug,
			siteapi.SiteSetupSteps{Integration: todo, SendingDomain: done, Contact: todo, SentEmail: done}, false},
		{"a fully set up Workspace is complete", fixtures.OwnerJohnEmail, fixtures.AcmeSlug,
			siteapi.SiteSetupSteps{Integration: done, SendingDomain: done, Contact: done, SentEmail: done}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := setupChecklist(t, tc.owner, tc.slug)
			assert.Equal(t, tc.want, got.Steps)
			assert.Equal(t, tc.complete, got.Complete)
			assert.False(t, got.Dismissed)
		})
	}
}
