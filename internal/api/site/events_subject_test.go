package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An email Event names the email it is about: the feed returns the subject its Outbound message
// kept, and none for an Event whose message kept none (#899).
func TestSiteEventsListNamesTheEmailAnEventIsAbout(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteEventsList(context.Background(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteEventsListOK)
	require.Truef(t, isOK, "got %T", res)

	byID := lo.KeyBy(ok.Items, func(e siteapi.SiteEventResource) siteapi.EntityId { return e.ID })
	opened, found := byID[siteapi.EntityId(fixtures.EventOpenedKeptSubjectID)]
	require.True(t, found, "the opened Event is on the page")
	subject, has := opened.EmailSubject.Get()
	assert.True(t, has)
	assert.Equal(t, "Welcome to Hexlet", subject)

	complained, found := byID[siteapi.EntityId(fixtures.EventComplainedKeptSubjectID)]
	require.True(t, found, "the complaint is on the page")
	subject, has = complained.EmailSubject.Get()
	assert.True(t, has)
	assert.Equal(t, "Welcome to Hexlet", subject)

	bounced, found := byID[siteapi.EntityId(fixtures.EventBouncedNoSubjectID)]
	require.True(t, found, "the bounced Event is on the page")
	_, has = bounced.EmailSubject.Get()
	assert.False(t, has, "a message that kept no subject names nothing")
}
