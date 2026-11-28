package downloadlinks_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/downloadlinks"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// payload is what a link names in these tests.
type payload struct {
	ImportID int64 `json:"importId"`
}

// A link is followed once, by the session it was issued to, for the download it was issued for:
// another session or purpose finds nothing and leaves it standing, and once spent it finds nothing
// again.
func TestALinkIsSpentOnceByItsSessionForItsPurpose(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	links := downloadlinks.New(env.FixturesNow)
	link, err := links.Issue(ctx, acme, downloadlinks.IssueInput{Purpose: downloadlinks.PurposeImportReport, Session: "session-a", Payload: payload{ImportID: 7}})
	require.NoError(t, err)
	follow := func(purpose downloadlinks.Purpose, session string) (payload, error) {
		var got payload
		err := links.Follow(ctx, acme, downloadlinks.FollowInput{Purpose: purpose, Token: link.Token, Session: session}, &got)
		return got, err
	}

	_, elsewhere := follow(downloadlinks.PurposeImportReport, "session-b")
	_, otherDownload := follow(downloadlinks.PurposeContactsExport, "session-a")
	got, err := follow(downloadlinks.PurposeImportReport, "session-a")
	_, again := follow(downloadlinks.PurposeImportReport, "session-a")

	require.ErrorIs(t, elsewhere, downloadlinks.ErrLinkInvalid)
	require.ErrorIs(t, otherDownload, downloadlinks.ErrLinkInvalid)
	require.NoError(t, err, "a refused follow leaves the link standing")
	assert.Equal(t, payload{ImportID: 7}, got)
	require.ErrorIs(t, again, downloadlinks.ErrLinkInvalid)
	assert.Equal(t, env.FixturesNow().Add(time.Minute), link.ExpiresAt, "a link stands a minute")
}

// A link lapses a minute after it was issued: following it then finds nothing.
func TestALinkLapses(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	now := env.FixturesNow()
	links := downloadlinks.New(func() time.Time { return now })
	link, err := links.Issue(ctx, acme, downloadlinks.IssueInput{Purpose: downloadlinks.PurposeContactsExport, Session: "session-a", Payload: payload{}})
	require.NoError(t, err)

	now = now.Add(time.Minute)
	var got payload
	err = links.Follow(ctx, acme, downloadlinks.FollowInput{Purpose: downloadlinks.PurposeContactsExport, Token: link.Token, Session: "session-a"}, &got)

	require.ErrorIs(t, err, downloadlinks.ErrLinkInvalid)
}
