package jobs_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// A client that LISTENs works a job another process inserted as soon as the insert commits: it
// does not wait for its poll, which with a listener runs only every few seconds. It runs on a
// database of its own, since a notification is only sent on commit.
func TestAListeningClientWorksAJobInsertedElsewhereAtOnce(t *testing.T) {
	ctx := context.Background()
	url := testhelper.RealDBURL(t)
	pool, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	listener, err := appdb.OpenRiverListener(ctx, url)
	require.NoError(t, err)
	t.Cleanup(listener.Close)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	entClient := appdb.NewEntClient(pool)
	bus := events.New(pool)
	mail := &testhelper.CapturingSender{}
	client, err := jobs.NewClient(riverdatabasesql.NewWithPgxListener(pool, listener), entClient,
		outbound.New(bus, nil, nil, tracking.New("test-secret", "http://local")), cipher, bus, mail, nil, nil,
		registry.Default(), cfg.AppURL, cfg.OutboundAllow, nil, jobs.Retention{})
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { cleanup.Report("stop the job client", client.Stop(context.Background())) })
	require.Eventually(t, func() bool {
		n, err := testhelper.Listeners(ctx, pool)
		return err == nil && n > 0
	}, 10*time.Second, 20*time.Millisecond, "the client LISTENs")

	elsewhere, err := jobs.NewInserter(pool)
	require.NoError(t, err)
	_, err = elsewhere.Insert(ctx, jobs.SendWelcomeArgs{Email: "listened@example.com", Name: "Listened"}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool { return sentTo(mail, "listened@example.com") }, 2*time.Second, 20*time.Millisecond,
		"worked well before the listener's fallback poll")
}

// sentTo reports whether the sender sent a message to the address.
func sentTo(mail *testhelper.CapturingSender, to string) bool {
	for _, m := range mail.Messages() {
		if m.To == to {
			return true
		}
	}
	return false
}
