package staff_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTheSignupSurveyCountsEveryAnswerIncludingOtherTexts(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res, err := c.StaffSignupSurveyCounts(t.Context())
	require.NoError(t, err)
	got, ok := res.(*staffeeapi.StaffSignupSurveyCounts)
	require.Truef(t, ok, "got %T", res)

	// Ann, Bob and Cara answered; every other fixture User is in no count.
	assert.EqualValues(t, 3, got.Responses)
	assert.Equal(t, []staffeeapi.StaffSignupRoleCount{
		{Choice: staffeeapi.StaffSignupRoleDeveloper, Count: 2},
		{Choice: staffeeapi.StaffSignupRoleOther, Count: 1},
	}, got.Role)
	assert.Equal(t, []staffeeapi.StaffSignupOtherTextCount{{Text: "Consultant", Count: 1}}, got.RoleOtherTexts)
	assert.Equal(t, []staffeeapi.StaffSignupCompanySizeCount{
		{Choice: staffeeapi.StaffSignupCompanySize1150, Count: 1},
		{Choice: staffeeapi.StaffSignupCompanySize210, Count: 1},
		{Choice: staffeeapi.StaffSignupCompanySizeOther, Count: 1},
	}, got.CompanySize)
	assert.Equal(t, []staffeeapi.StaffSignupOtherTextCount{{Text: "Just me and a cat", Count: 1}}, got.CompanySizeOtherTexts)
	assert.Equal(t, []staffeeapi.StaffSignupGoalCount{
		{Choice: staffeeapi.StaffSignupGoalBroadcasts, Count: 1},
		{Choice: staffeeapi.StaffSignupGoalMigrating, Count: 2},
	}, got.MainGoal)
	assert.Empty(t, got.MainGoalOtherTexts)
	// Bob skipped the optional question: he is absent from its counts.
	assert.Equal(t, []staffeeapi.StaffSignupSourceCount{{Choice: staffeeapi.StaffSignupSourceOther, Count: 2}}, got.HeardFrom)
	assert.Equal(t, []staffeeapi.StaffSignupOtherTextCount{{Text: "A newsletter", Count: 2}}, got.HeardFromOtherTexts)
}

func TestAUsersSignupSurveyAnswersAreShownToAStaffMember(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res, err := c.StaffSignupSurveyGet(t.Context(), staffeeapi.StaffSignupSurveyGetParams{UserId: fixtures.SurveyAnsweredAnnID})
	require.NoError(t, err)
	got, ok := res.(*staffeeapi.StaffSignupSurveyResponseResource)
	require.Truef(t, ok, "got %T", res)

	assert.EqualValues(t, fixtures.SurveyAnsweredAnnID, got.UserId)
	assert.Equal(t, staffeeapi.StaffSignupRoleDeveloper, got.Role)
	assert.Equal(t, staffeeapi.StaffSignupCompanySize1150, got.CompanySize)
	assert.Equal(t, staffeeapi.StaffSignupGoalMigrating, got.MainGoal)
	assert.Equal(t, staffeeapi.StaffSignupSourceOther, got.HeardFrom.Value)
	assert.Equal(t, "A newsletter", got.HeardFromOtherText.Value)
	assert.False(t, got.RoleOtherText.IsSet())
}

func TestASkippedOptionalQuestionIsAbsentFromAUsersAnswers(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res, err := c.StaffSignupSurveyGet(t.Context(), staffeeapi.StaffSignupSurveyGetParams{UserId: fixtures.SurveyAnsweredBobID})
	require.NoError(t, err)
	got, ok := res.(*staffeeapi.StaffSignupSurveyResponseResource)
	require.Truef(t, ok, "got %T", res)

	assert.False(t, got.HeardFrom.IsSet())
}

func TestAUserWhoHasNotAnsweredTheSignupSurveyHasNoAnswers(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	for _, id := range []int64{fixtures.OwnerJohnID, fixtures.MemberMaryID} {
		res, err := c.StaffSignupSurveyGet(t.Context(), staffeeapi.StaffSignupSurveyGetParams{UserId: staffeeapi.EntityId(id)})
		require.NoError(t, err)
		assert.IsTypef(t, new(staffeeapi.StaffSignupSurveyGetNotFound), res, "user %d", id)
	}
}

func TestTheSignupSurveyReadsNeedAStaffSession(t *testing.T) {
	h := newHarness(t)

	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, "/staff/signup-survey").Code)
	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, "/staff/signup-survey/users/5600").Code)
}

func TestTheSignupSurveyReadsAnswer404WithoutTheStaffLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, "/staff/signup-survey").Code)
	assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, "/staff/signup-survey/users/5600").Code)
}
