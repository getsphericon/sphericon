package suppressionsunchecked_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions/suppressionsunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestEraseContactKeepsTheSuppressionRefusingTheAddress(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.InitechID)

	require.NoError(t, suppressionsunchecked.EraseContact(ctx, s, int64(fixtures.ContactErasableID), ""))

	row, err := s.Suppression().Get(ctx, fixtures.SuppressionErasableID)
	require.NoError(t, err)
	assert.Nil(t, row.LiftedAt)
	verdict, err := eligibility.Check(ctx, s, eligibility.ChannelEmail, row.Destination, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, verdict.Eligible)
}
