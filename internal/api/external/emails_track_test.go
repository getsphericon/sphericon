package external_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Transactional send may opt into tracking (ADR 0053): the open pixel and the rewritten links,
// and nothing else: it stays Transactional, with no unsubscribe link.

const orderLink = "https://shop.test/order"

var (
	clickHref = regexp.MustCompile(`/e/c/[A-Za-z0-9._-]+`)
	openSrc   = regexp.MustCompile(`/e/o/[A-Za-z0-9._-]+`)
)

func sendLinked(t *testing.T, env *testhelper.TestEnv, dest string, track externalapi.OptBool) externalapi.EmailsSendRes {
	t.Helper()
	res, err := env.ExternalScoped(t, "emails:send").EmailsSend(t.Context(), &externalapi.SendTransactionalEmailInput{
		TemplateId:  externalapi.EntityId(fixtures.TemplateWithLinkID),
		Destination: externalapi.EmailAddress(dest),
		Track:       track,
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	return res
}

func get(t *testing.T, env *testhelper.TestEnv, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestTransactionalSendWithoutTrackIsNotRewritten(t *testing.T) {
	env := testhelper.Setup(t)

	res := sendLinked(t, env, fixtures.ContactLiamEmail, externalapi.OptBool{})
	require.IsType(t, &externalapi.SendTransactionalEmailResponse{}, res)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, orderLink)
	assert.NotContains(t, msgs[0].HTML, "/e/c/")
	assert.NotContains(t, msgs[0].HTML, "/e/o/")
}

func TestTrackedTransactionalSendRecordsOpenAndClickAndStaysTransactional(t *testing.T) {
	env := testhelper.Setup(t)

	res := sendLinked(t, env, fixtures.ContactLiamEmail, externalapi.NewOptBool(true))
	require.IsType(t, &externalapi.SendTransactionalEmailResponse{}, res)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	html := msgs[0].HTML
	assert.NotContains(t, html, "/e/u/", "a tracked Transactional send carries no unsubscribe link")
	assert.NotContains(t, strings.ToLower(html), "unsubscribe")
	assert.Empty(t, msgs[0].ListUnsubscribeURL)

	click := clickHref.FindString(html)
	pixel := openSrc.FindString(html)
	require.NotEmpty(t, click)
	require.NotEmpty(t, pixel)

	assert.Equal(t, http.StatusOK, get(t, env, pixel).Code)
	redirect := get(t, env, click)
	assert.Equal(t, http.StatusFound, redirect.Code)
	assert.Equal(t, orderLink, redirect.Header().Get("Location"))

	opened := env.OutboxEvents(t, events.NameEmailOpened)
	require.Len(t, opened, 1)
	assert.EqualValues(t, fixtures.ContactLiamID, opened[0].(*events.EmailEngagement).ContactID)
	clicked := env.OutboxEvents(t, events.NameEmailClicked)
	require.Len(t, clicked, 1)
	assert.Equal(t, orderLink, clicked[0].(*events.EmailEngagement).URL)
}

func TestTrackedTransactionalSendToAnAddressWithNoContactRecordsByAddress(t *testing.T) {
	env := testhelper.Setup(t)
	dest := fixtures.UnsubscribeEverythingNoContactDestination

	require.IsType(t, &externalapi.SendTransactionalEmailResponse{}, sendLinked(t, env, dest, externalapi.NewOptBool(true)))
	pixel := openSrc.FindString(env.CustomerMail.Messages()[0].HTML)
	require.NotEmpty(t, pixel)
	assert.Equal(t, http.StatusOK, get(t, env, pixel).Code)

	opened := env.OutboxEvents(t, events.NameEmailOpened)
	require.Len(t, opened, 1)
	got := opened[0].(*events.EmailEngagement)
	assert.Zero(t, got.ContactID)
	assert.Equal(t, dest, got.Email)
}

func TestTrackedTransactionalSendToASuppressedAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	res := sendLinked(t, env, fixtures.SuppressionGhostBounceDestination, externalapi.NewOptBool(true))
	ok, isOK := res.(*externalapi.SendTransactionalEmailResponse)
	require.True(t, isOK)
	assert.Equal(t, externalapi.TransactionalSendStatusSuppressed, ok.Status)
	assert.Empty(t, env.CustomerMail.Messages())
}
