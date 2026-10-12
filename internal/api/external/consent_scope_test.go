package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An automation of another Workspace is not a valid sendingSource: 422, and no
// Unsubscribe row is written.
func TestExternalUnsubscribesCreateRejectsForeignAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	before, err := env.DB.Unsubscribe.Query().Count(context.Background())
	require.NoError(t, err)

	res, err := c.UnsubscribesCreate(context.Background(), &externalapi.CreateUnsubscribeInput{
		Destination:   fixtures.ContactAliceEmail,
		SendingSource: externalapi.NewOptString(eligibility.AutomationSource(fixtures.AutomationGlobexID)),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.UnsubscribesCreateUnprocessableEntity{}, res)

	after, err := env.DB.Unsubscribe.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
