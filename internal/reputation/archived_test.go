package reputation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An archived Sending domain still has its rates: archiving never changes past figures.
func TestRatesIncludeArchivedDomains(t *testing.T) {
	env := testhelper.Setup(t)
	rates, err := reputation.New().Rates(context.Background(), env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
	require.NoError(t, err)

	byName := byDomain(t, rates)
	_, listed := byName[fixtures.SendingDomainArchivedDomain]
	assert.True(t, listed, "the archived domain keeps its rates")
}
