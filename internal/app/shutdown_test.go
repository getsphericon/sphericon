package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A job still running when the app shuts down is cancelled after the soft stop and recorded as
// interrupted: available again, with its attempt given back. Close, within the budget the server
// binary gives it, returns only once that is written. The job is a webhook delivery to an endpoint
// that never answers, which outlasts the soft stop.
func TestCloseGivesARunningJobItsAttemptBack(t *testing.T) {
	require.Greater(t, webhook.DeliveryTimeout, jobs.SoftStopTimeout,
		"the delivery must still be running when the soft stop cancels it")
	// The delivery the endpoint holds, by the id the delivery's request carries.
	reached := make(chan string, 1)
	// The server does not always see the client give up, and its Close waits for the handler, so
	// the test lets the handler go before it closes the server (cleanups run last first).
	release := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case reached <- r.Header.Get(standardwebhooks.HeaderWebhookID):
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(endpoint.Close)
	t.Cleanup(func() { close(release) })
	t.Setenv("SPHERICON_OUTBOUND_ALLOW", endpoint.Listener.Addr().String())

	a, _ := lifecycleApp(t)
	require.NoError(t, a.Start(context.WithoutCancel(t.Context())))
	t.Cleanup(func() { cleanup.Report("close the app", a.Close(closeCtx(t))) })

	client := externalClient(t, a)
	// The endpoint that never answers is the Workspace's only one, so the Event has one delivery.
	others, err := client.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	require.NoError(t, err)
	for _, w := range others.Items {
		_, err := client.WebhooksUpdate(t.Context(), &externalapi.UpdateWebhookInput{
			Revision: w.Revision, Enabled: externalapi.NewOptBool(false),
		}, externalapi.WebhooksUpdateParams{ID: w.ID})
		require.NoError(t, err)
	}
	_, err = client.WebhooksCreate(t.Context(), &externalapi.CreateWebhookInput{
		URL: endpoint.URL, EventTypes: []string{systemevent.ContactCreated},
	})
	require.NoError(t, err)
	_, err = client.ContactsCreate(t.Context(), &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("interrupted@example.com"),
	})
	require.NoError(t, err)
	held := testhelper.Receive(t, reached, "the delivery never reached the endpoint")

	ctx, cancel := context.WithTimeout(t.Context(), app.StopTimeout)
	defer cancel()
	require.NoError(t, a.Close(ctx))

	pool, err := db.OpenRiverDB(a.Config.DatabaseURL, a.Config.DBPool)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	res, err := testhelper.RiverClientOn(t, pool).JobList(t.Context(), river.NewJobListParams().Kinds(jobkind.DeliverWebhook.Kind))
	require.NoError(t, err)
	require.Len(t, res.Jobs, 1)
	job := res.Jobs[0]
	var args jobs.DeliverWebhookArgs
	require.NoError(t, json.Unmarshal(job.EncodedArgs, &args))
	require.Equal(t, held, args.DeliveryID, "the job is the delivery the endpoint held")
	assert.Equal(t, rivertype.JobStateAvailable, job.State)
	assert.Equal(t, 0, job.Attempt, "the interrupted run costs no attempt")
}
