package site_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// unknownPathOf asks whether a Branch whose listed paths hold these Conditions needs its unknown
// path, as the editor asks while a Condition is edited.
func unknownPathOf(t *testing.T, conditions ...string) bool {
	t.Helper()
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	in := siteapi.BranchUnknownPathInput{Conditions: make([]siteapi.BranchConditionInput, len(conditions))}
	for i, doc := range conditions {
		require.NoError(t, json.Unmarshal([]byte(doc), &in.Conditions[i]))
	}
	res, err := c.SiteAutomationsUnknownPath(t.Context(), &in,
		siteapi.SiteAutomationsUnknownPathParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res.Needed
}

const (
	openedOnceCondition = `{"kind":"group","combinator":"and","entries":[{"kind":"event","action":"email.opened","operator":"atLeast","times":1}]}`
	vipTagCondition     = `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}`
	ownMessageCondition = `{"kind":"group","combinator":"and","entries":[{"kind":"group","combinator":"and","not":true,"entries":[{"kind":"stepMessage","step":"5a0f6d2e-8b1c-4e3a-9f7d-000000000002","engagement":"opened"}]}]}`
	orderCondition      = `{"kind":"group","combinator":"and","entries":[{"kind":"property","of":{"kind":"trigger"},"key":"total","test":{"kind":"number","operator":"gt","value":100}}]}`
)

// A Branch needs an unknown path when one of its Conditions counts an open or a click (ADR 0123,
// ADR 0124): the server answers it for the Conditions being edited, as Publish decides it.
func TestSiteAutomationBranchNeedsAnUnknownPathWhenAConditionCountsAnOpen(t *testing.T) {
	needed := unknownPathOf(t, vipTagCondition, openedOnceCondition)

	assert.True(t, needed)
}

// A test of the Enrollment's own message can be unknown too (ADR 0122), so a Branch that holds one
// needs an unknown path.
func TestSiteAutomationBranchNeedsAnUnknownPathWhenAConditionTestsItsOwnMessage(t *testing.T) {
	needed := unknownPathOf(t, vipTagCondition, ownMessageCondition)

	assert.True(t, needed)
}

func TestSiteAutomationBranchNeedsNoUnknownPathWithoutAnEngagementTest(t *testing.T) {
	needed := unknownPathOf(t, vipTagCondition, orderCondition)

	assert.False(t, needed)
}
