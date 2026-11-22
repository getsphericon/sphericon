package testhelper_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A committed env is the Instance over a database whose writes really commit: what a transaction of
// its bus writes is seen by the env's other connections once it commits, and not before.
func TestACommittedEnvsWriteIsSeenByAnotherConnectionOnceItCommits(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	s := env.DB.Scoped(fixtures.AcmeID)
	const email = "committed@example.com"
	seen := func() bool {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		found, err := s.Contact().Query().Where(contact.Email(email)).Exist(ctx)
		require.NoError(t, err, "another connection reads while the write is open")
		return found
	}

	release := env.HoldOpen(t, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// test writes: a row no other connection may see before its transaction commits
		return ts.Contact().Create().SetSource(contact.SourceManual).SetEmail(email).Exec(t.Context())
	})
	assert.False(t, seen(), "the open transaction's row is its own")

	release()
	assert.True(t, seen(), "the committed row is everyone's")
}

// The jobs a committed env's publish queues wait in its queue, committed, and the harness works
// them as it works a rolled-back env's: the Notice a published request calls for is sent once its
// jobs are worked.
func TestACommittedEnvWorksTheJobsItsPublishQueued(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())

	require.NoError(t, env.Bus.Publish(t.Context(), &events.LinkRequested{
		Action: events.NameUserPasswordResetRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail, Deliver: true,
	}))

	env.PerformEnqueuedJobs(t)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
}
