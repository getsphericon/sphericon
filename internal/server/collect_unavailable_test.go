package server_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"entgo.io/ent"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entgen "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// failVisitorWrites makes the transaction collect opens fail the way a lost database does.
func failVisitorWrites(env *testhelper.TestEnv, err error) {
	env.Bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Type() == entgen.TypeVisitor {
				return nil, err
			}
			return next.Mutate(ctx, m)
		})
	})
}

func TestCollectAnswers503WithRetryAfterWhenTheDatabaseIsUnavailable(t *testing.T) {
	env := testhelper.Setup(t)
	failVisitorWrites(env, &pgconn.ConnectError{})

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameCollected), "the request changed nothing")
}

// A connection lost mid-write may or may not have stored the event; a retry is counted once
// (clientEventId), so the client is told to send it again.
func TestCollectAnswers503WithRetryAfterWhenTheConnectionIsLostMidRequest(t *testing.T) {
	env := testhelper.Setup(t)
	failVisitorWrites(env, fmt.Errorf("insert visitor: %w", pgconn.ErrConnClosed))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestCollectKeepsAnswering500ForAFailureThatIsNotTheDatabaseBeingGone(t *testing.T) {
	env := testhelper.Setup(t)
	failVisitorWrites(env, errors.New("injected failure"))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, rec.Header().Get("Retry-After"))
}
