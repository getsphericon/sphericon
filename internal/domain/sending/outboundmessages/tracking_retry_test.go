package outboundmessages_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// setAcmeDefault sets Acme's Engagement tracking default.
func setAcmeDefault(t *testing.T, env *testhelper.TestEnv, d workspace.EngagementTrackingDefault) {
	t.Helper()
	_, err := testhelper.Module[*workspaces.Module](env).SetEngagementTrackingDefault(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID)), d)
	require.NoError(t, err)
}

// A retry renders tracked only when the attempt before it did (ADR 0120): a message whose first
// attempt rendered untracked under dont_track and failed at the provider goes out untracked though
// the default turned back to track meanwhile.
func TestARetryOfAnAttemptThatRenderedUntrackedStaysUntracked(t *testing.T) {
	env := testhelper.Setup(t)
	mod := newModule(env, time.Now)
	req := marketing(t, env, "retry-untracked", fixtures.ContactAliceID)
	req.Track = true
	s := env.DB.Scoped(fixtures.AcmeID)

	setAcmeDefault(t, env, workspace.EngagementTrackingDefaultDontTrack)
	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := mod.Send(t.Context(), s, req)
	require.Error(t, err, "the provider failed: the send is retried")
	env.CustomerMail.SetErr(nil)
	setAcmeDefault(t, env, workspace.EngagementTrackingDefaultTrack)
	res, err := mod.Send(t.Context(), s, req)

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.NotContains(t, msgs[0].HTML, "/e/o/", "the retry carries no pixel")
	assert.Contains(t, msgs[0].HTML, "/e/u/", "and keeps its unsubscribe footer")
}

// A retry of an attempt that rendered tracked renders tracked again while the Destination still
// consents.
func TestARetryOfAnAttemptThatRenderedTrackedStaysTracked(t *testing.T) {
	env := testhelper.Setup(t)
	mod := newModule(env, time.Now)
	req := marketing(t, env, "retry-tracked", fixtures.ContactAliceID)
	req.Track = true
	s := env.DB.Scoped(fixtures.AcmeID)

	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := mod.Send(t.Context(), s, req)
	require.Error(t, err, "the provider failed: the send is retried")
	env.CustomerMail.SetErr(nil)
	_, err = mod.Send(t.Context(), s, req)

	require.NoError(t, err)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, "http://local/e/o/", "the retry carries the pixel")
}
