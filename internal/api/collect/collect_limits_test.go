package collect_test

import (
	"context"
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

	res, err := env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(100))
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)

	// ogen validates the decoded body and answers the contract's 400 before a handler runs.
	res, err = env.CollectAcme(t).CollectEventsCreate(ctx, eventBatch(101))
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateBadRequest{}, res)
}

func TestCollectEventFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	send := func(visitorID, action string) collectapi.CollectEventsCreateRes {
		res, err := c.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
			Events: []collectapi.CollectEventInput{{VisitorId: visitorID, Action: action}},
		})
		require.NoError(t, err)
		return res
	}
	rejected := &collectapi.CollectEventsCreateBadRequest{}

	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, send(strings.Repeat("v", 255), strings.Repeat("a", 255)), "at the limit")
	assert.IsType(t, rejected, send(strings.Repeat("v", 256), "view"))
	assert.IsType(t, rejected, send("dev-1", strings.Repeat("a", 256)))
	assert.IsType(t, rejected, send("", "view"))
	assert.IsType(t, rejected, send("dev-1", ""))
}

func TestCollectIdentifyFieldsAreBoundedInLength(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.CollectAcme(t)

	identify := func(in collectapi.CollectIdentifyInput) collectapi.CollectIdentifyCreateRes {
		res, err := c.CollectIdentifyCreate(ctx, &in)
		require.NoError(t, err)
		return res
	}
	rejected := &collectapi.CollectIdentifyCreateBadRequest{}
	assert.IsType(t, &collectapi.CollectOkResponse{}, identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 255),
		Phone:     collectapi.NewOptNilString(strings.Repeat("1", 32)),
		SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 255)),
	}), "at the limit")

	assert.IsType(t, rejected, identify(collectapi.CollectIdentifyInput{
		VisitorId: strings.Repeat("v", 256),
		SubjectId: collectapi.NewOptNilString("crm-1"),
	}))
	assert.IsType(t, rejected, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Phone: collectapi.NewOptNilString(strings.Repeat("1", 33)),
	}))
	assert.IsType(t, rejected, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", SubjectId: collectapi.NewOptNilString(strings.Repeat("s", 256)),
	}))
	assert.IsType(t, rejected, identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-id", Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(strings.Repeat("a", 321-len("@example.com")+1) + "@example.com")),
	}))

	n, err := env.DB.Contact.Query().Where(contact.Phone(strings.Repeat("1", 33))).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a rejected identify creates no contact")
}
