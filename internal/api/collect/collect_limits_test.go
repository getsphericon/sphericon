package collect_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

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

	err := env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(100))
	require.NoError(t, err)

	// ogen validates the decoded body and answers the contract's 400 before a handler runs.
	err = env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(101))
	testhelper.CollectRefusal(t, err, http.StatusBadRequest)
}

func TestCollectEventFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	send := func(visitorID, action string) error {
		return c.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
			Events: []collectapi.CollectEventInput{{VisitorId: visitorID, Action: action}},
		})
	}
	rejected := func(err error) { t.Helper(); testhelper.CollectRefusal(t, err, http.StatusBadRequest) }

	require.NoError(t, send(strings.Repeat("v", 255), strings.Repeat("a", 255)), "at the limit")
	rejected(send(strings.Repeat("v", 256), "view"))
	rejected(send("dev-1", strings.Repeat("a", 256)))
	rejected(send("", "view"))
	rejected(send("dev-1", ""))
}

func TestCollectIdentifyFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	identify := func(in collectapi.CollectIdentifyInput) error {
		_, err := c.CollectIdentifyCreate(ctx, &in)
		return err
	}
	rejected := func(err error) { t.Helper(); testhelper.CollectRefusal(t, err, http.StatusBadRequest) }
	require.NoError(t, identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 255),
		// 32 characters: a valid number, its digits spaced out.
		Phone:     collectapi.NewOptNilString("+1" + strings.Repeat(" ", 18) + "415 555 0118"),
		SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 255)),
	}), "at the limit")

	rejected(identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 256),
		SubjectId: collectapi.NewOptNilString("crm-1"),
	}))
	rejected(identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Phone: collectapi.NewOptNilString(strings.Repeat("1", 33)),
	}))
	rejected(identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 256)),
	}))
	rejected(identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(strings.Repeat("a", 321-len("@example.com")+1) + "@example.com")),
	}))

	n, err := env.DB.Contact.Query().Where(contact.Phone(strings.Repeat("1", 33))).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a rejected identify creates no contact")
}
