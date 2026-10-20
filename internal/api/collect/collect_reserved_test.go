package collect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A tracked event never claims one of the system's actions, and the whole batch is refused.
func TestCollectRefusesReservedActions(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{
			{VisitorId: "v-1", Action: "page_view"},
			{VisitorId: "v-1", Action: "email.opened"},
		},
	})
	require.NoError(t, err)
	refused, isRefused := res.(*collectapi.CollectEventsCreateUnprocessableEntity)
	require.Truef(t, isRefused, "got %T", res)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, "reserved")
	assert.Empty(t, env.OutboxEnvelopes(t), "nothing of the batch is published")
}
