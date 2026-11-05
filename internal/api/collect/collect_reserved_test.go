package collect_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A tracked event never claims one of the system's actions, and the whole batch is refused.
func TestCollectRefusesReservedActions(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{
			{VisitorId: "v-1", Action: "page_view"},
			{VisitorId: "v-1", Action: "email.opened"},
			{VisitorId: "v-1", Action: "telegram.bound"},
		},
	})
	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, "reserved")
	assert.Empty(t, env.QueuedEnvelopes(t), "nothing of the batch is published")
}

// The sms. namespace is the system's: a tracked event cannot claim a text was received, delivered
// or clicked.
func TestCollectRefusesSMSActions(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{VisitorId: "v-1", Action: "sms.received"}},
	})
	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "reserved")
	assert.Empty(t, env.QueuedEnvelopes(t), "nothing is published")
}
