package external_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	ht "github.com/ogen-go/ogen/http"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// recorder keeps every response body its client reads.
type recorder struct {
	inner  ht.Client
	bodies *[]string
}

func (r recorder) Do(req *http.Request) (*http.Response, error) {
	resp, err := r.inner.Do(req)
	if err != nil {
		return resp, err
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	*r.bodies = append(*r.bodies, req.URL.String()+" "+string(raw))
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}

// No key reaches a client (ADR 0142): with the Contacts' keys marked by a number nothing else
// holds, a Contact created, tagged and read through the API shows up in the responses, the Audit
// log, the Export streams and their cursors, and the webhook deliveries it queues, and the number
// is in none of them.
func TestNoContactKeyReachesAClient(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const mark = 7770000001
	testhelper.RestartIDs(t, env.SQLDB, "contacts", mark)
	bodies := make([]string, 0, 16)
	c := env.ExternalVia(t, env.ScopedBearerFor(t, fixtures.AcmeID, apitokens.Scopes...), func(inner ht.Client) ht.Client {
		return recorder{inner: inner, bodies: &bodies}
	})

	created, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("marked@example.com")})
	require.NoError(t, err)
	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "marked"}, externalapi.TagsApplyParams{ContactId: created.ID})
	require.NoError(t, err)
	_, err = c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: created.ID})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, append([]river.JobArgs{jobs.WebhookEventArgs{}}, testhelper.EditionJobs...)...)
	_, err = c.AuditEntriesList(ctx, externalapi.AuditEntriesListParams{})
	require.NoError(t, err)
	for _, stream := range []exportStream{contactsStream, eventsStream, auditStream} {
		body, next, err := stream.call(ctx, c, exportParams{})
		require.NoErrorf(t, err, "%s", stream.name)
		raw, err := io.ReadAll(body)
		require.NoError(t, err)
		bodies = append(bodies, stream.name+" cursor "+next+" "+string(raw))
	}
	for _, d := range env.QueuedDeliveries(t) {
		bodies = append(bodies, "webhook "+string(d.Body))
	}

	assert.Contains(t, strings.Join(bodies, "\n"), "marked@example.com", "the Contact was read back")
	for _, b := range bodies {
		assert.NotContains(t, b, "777000000", "a key reached a client: %.300s", b)
	}
}
