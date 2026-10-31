package events_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const poisonEmail = "poison@example.com"

// A handler that fails on an event, by an error or a panic, is retried a bounded number of
// times; then the event goes to the dead letters with its group, its error and its envelope,
// and the group goes on to the next event. Replayed to its group once the handler accepts it,
// the dead letter is gone.
func TestAFailingHandlerDeadLettersTheEventAndItsGroupMovesOn(t *testing.T) {
	for _, c := range []struct {
		name   string
		fail   func() error
		reason string
	}{
		{"an error", func() error { return errors.New("the handler refused it") }, "the handler refused it"},
		{"a panic", func() error { panic("the handler crashed") }, "the handler crashed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			sqlDB := testhelper.RealDB(t)
			client := db.NewEntClient(sqlDB)
			attempts := 0
			got := make(chan string, 1)
			handle := func(msg *message.Message) error {
				email := emailOf(msg)
				if email == poisonEmail {
					attempts++
					return c.fail()
				}
				got <- email
				return nil
			}
			runRouter(t, sqlDB, events.ConsumerHandlers{{Group: "flaky", Name: "flaky", Handle: handle}})

			publish(ctx, t, sqlDB, poisonEmail)
			publish(ctx, t, sqlDB, "next@example.com")

			assert.Equal(t, "next@example.com", testhelper.Receive(t, got, "the group moved on past the poisoned event"))
			assert.Equal(t, 3, attempts, "the first attempt and two retries")
			letters := client.OutboxDeadLetter.Query().AllX(ctx)
			require.Len(t, letters, 1)
			var metadata map[string]string
			require.NoError(t, json.Unmarshal(letters[0].Metadata, &metadata))
			assert.Equal(t, "flaky", metadata[events.MetadataConsumerGroup])
			assert.Contains(t, metadata[middleware.ReasonForPoisonedKey], c.reason)
			var env events.Envelope
			require.NoError(t, json.Unmarshal(letters[0].Payload, &env))
			assert.Equal(t, events.NameContactCreated, env.Name)

			replayed, err := events.ReplayDeadLetters(ctx, client, "flaky", func(*message.Message) error { return nil })
			require.NoError(t, err)
			assert.Equal(t, 1, replayed)
			assert.Zero(t, client.OutboxDeadLetter.Query().CountX(ctx))
		})
	}
}

// A replay stops at a dead letter its group's handler still refuses, and keeps it; another
// group's dead letters are not its to replay.
func TestAReplayKeepsWhatItsGroupStillRefuses(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	client := db.NewEntClient(sqlDB)
	runRouter(t, sqlDB, events.ConsumerHandlers{
		{Group: "flaky", Name: "flaky", Handle: func(*message.Message) error { return errors.New("refused") }},
		{Group: "other", Name: "other", Handle: func(*message.Message) error { return errors.New("refused") }},
	})
	publish(ctx, t, sqlDB, poisonEmail)
	require.Eventually(t, func() bool { return client.OutboxDeadLetter.Query().CountX(ctx) == 2 }, 10*time.Second, 20*time.Millisecond)

	replayed, err := events.ReplayDeadLetters(ctx, client, "flaky", func(*message.Message) error { return errors.New("still refused") })

	require.ErrorContains(t, err, "still refused")
	assert.Zero(t, replayed)
	assert.Equal(t, 2, client.OutboxDeadLetter.Query().CountX(ctx))
}

// runRouter runs a router with these handlers and a short retry policy until the test ends.
func runRouter(t *testing.T, sqlDB *sql.DB, handlers events.ConsumerHandlers) {
	t.Helper()
	router, err := events.NewRouter(sqlDB, events.WithRetry(middleware.Retry{MaxRetries: 2, InitialInterval: time.Millisecond}))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(router, "router") })
	require.NoError(t, events.RegisterSubscribers(router, sqlDB, handlers, testhelper.PrivateOutbox()))
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { cleanup.Report("run the router", router.Run(runCtx)) }()
	<-router.Running()
}

func publish(ctx context.Context, t *testing.T, sqlDB *sql.DB, email string) {
	t.Helper()
	require.NoError(t, events.New(sqlDB).WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: email})
	}))
}

// emailOf is the address of the ContactCreated event msg carries, empty for anything else.
func emailOf(msg *message.Message) string {
	var env events.Envelope
	if err := json.Unmarshal(msg.Payload, &env); err != nil {
		return ""
	}
	ev, err := events.Decode(env)
	if err != nil {
		return ""
	}
	return ev.Project().Email
}
