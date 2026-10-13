package collect_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ogen-go/ogen/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ogen validates the decoded body and answers 400, which the contract does not declare,
// so the client surfaces it as an unexpected status.
func requireRejectedByValidation(t *testing.T, err error) {
	t.Helper()
	var status *validate.UnexpectedStatusCodeError
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusBadRequest, status.StatusCode)
}

func eventBatch(n int) *collectapi.CollectEventsInput {
	batch := make([]collectapi.CollectEventInput, n)
	for i := range batch {
		batch[i] = collectapi.CollectEventInput{VisitorId: "dev-batch", Action: "view"}
	}
	return &collectapi.CollectEventsInput{Events: batch}
}

func TestCollectEventsBatchIsCappedAt100(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(100))
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)

	_, err = env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(101))
	requireRejectedByValidation(t, err)
}

func TestCollectEventFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	send := func(visitorID, action string) error {
		_, err := c.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
			Events: []collectapi.CollectEventInput{{VisitorId: visitorID, Action: action}},
		})
		return err
	}

	require.NoError(t, send(strings.Repeat("v", 255), strings.Repeat("a", 255)), "at the limit")
	requireRejectedByValidation(t, send(strings.Repeat("v", 256), "view"))
	requireRejectedByValidation(t, send("dev-1", strings.Repeat("a", 256)))
	requireRejectedByValidation(t, send("", "view"))
	requireRejectedByValidation(t, send("dev-1", ""))
}

func TestCollectIdentifyFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	identify := func(in collectapi.CollectIdentifyInput) error {
		_, err := c.CollectIdentifyCreate(ctx, &in)
		return err
	}
	require.NoError(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 255),
		Phone:     collectapi.NewOptNilString(strings.Repeat("1", 32)),
		SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 255)),
	}), "at the limit")

	requireRejectedByValidation(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 256),
		SubjectId: collectapi.NewOptNilString("crm-1"),
	}))
	requireRejectedByValidation(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Phone: collectapi.NewOptNilString(strings.Repeat("1", 33)),
	}))
	requireRejectedByValidation(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 256)),
	}))
	requireRejectedByValidation(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(strings.Repeat("a", 321-len("@example.com")+1) + "@example.com")),
	}))

	n, err := env.DB.Contact.Query().Where(contact.Phone(strings.Repeat("1", 33))).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a rejected identify creates no contact")
}
