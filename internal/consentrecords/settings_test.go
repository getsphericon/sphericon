package consentrecords_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/internal/consentrecords"
)

// answered is a current answer to the news item.
func answered(decision consentrecord.Decision, version string) []consentrecords.Record {
	return []consentrecords.Record{{Key: news.Key, Version: version, Decision: decision}}
}

func TestGivingAnOptionalConsentRecordsItAcceptedOnTheCurrentVersion(t *testing.T) {
	entry, err := consentrecords.ChangeOptional(items, answered(consentrecord.DecisionDeclined, news.Label), news.Key, news.Label, true)

	require.NoError(t, err)
	assert.Equal(t, &consentrecords.Entry{VersionID: news.VersionID, Decision: consentrecord.DecisionAccepted}, entry)
}

func TestWithdrawingAGivenConsentRecordsItWithdrawn(t *testing.T) {
	entry, err := consentrecords.ChangeOptional(items, answered(consentrecord.DecisionAccepted, "v0"), news.Key, news.Label, false)

	require.NoError(t, err)
	assert.Equal(t, &consentrecords.Entry{VersionID: news.VersionID, Decision: consentrecord.DecisionWithdrawn}, entry)
}

func TestAConsentNeverAnsweredIsGivenAndOneNeverGivenHasNothingToWithdraw(t *testing.T) {
	given, err := consentrecords.ChangeOptional(items, nil, news.Key, news.Label, true)
	require.NoError(t, err)
	withdrawn, err := consentrecords.ChangeOptional(items, nil, news.Key, news.Label, false)
	require.NoError(t, err)

	assert.Equal(t, &consentrecords.Entry{VersionID: news.VersionID, Decision: consentrecord.DecisionAccepted}, given)
	assert.Nil(t, withdrawn)
}

func TestOnlyAnOptionalConsentInEffectAtItsCurrentVersionIsChanged(t *testing.T) {
	cases := map[string]struct {
		key, version string
		refusal      error
	}{
		"required acceptance": {terms.Key, terms.Label, consentrecords.ErrNotOptional},
		"notice":              {notice.Key, notice.Label, consentrecords.ErrNotOptional},
		"not in effect":       {"gone", "v1", consentrecords.ErrNotInEffect},
		"stale version":       {news.Key, "v0", consentrecords.ErrStaleVersion},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			entry, err := consentrecords.ChangeOptional(items, nil, c.key, c.version, true)

			require.ErrorIs(t, err, c.refusal)
			assert.Nil(t, entry)
		})
	}
}

// Whatever the User answered before, the consent is given after a change exactly when the change
// gives it, and a change records nothing when the consent is already as asked at the current
// version.
func TestAChangeLeavesTheConsentGivenExactlyWhenItGivesIt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawAnswer(t)
		accepted := rapid.Bool().Draw(t, "accepted")

		entry, err := consentrecords.ChangeOptional(items, before, news.Key, news.Label, accepted)
		require.NoError(t, err)

		assert.Equal(t, accepted, givenAfter(before, entry))
		assert.Equal(t, alreadyAsAsked(before, accepted), entry == nil)
	})
}

// drawAnswer is a current answer to the news item, at its current version or an older one, or
// none.
func drawAnswer(t *rapid.T) []consentrecords.Record {
	if !rapid.Bool().Draw(t, "answered") {
		return nil
	}
	decision := rapid.SampledFrom([]consentrecord.Decision{
		consentrecord.DecisionAccepted, consentrecord.DecisionDeclined, consentrecord.DecisionWithdrawn,
	}).Draw(t, "decision")
	return answered(decision, rapid.SampledFrom([]string{news.Label, "v0"}).Draw(t, "version"))
}

// givenAfter is whether the consent is given once the entry, if any, is recorded.
func givenAfter(before []consentrecords.Record, entry *consentrecords.Entry) bool {
	if entry != nil {
		return entry.Decision == consentrecord.DecisionAccepted
	}
	return len(before) == 1 && before[0].Decision == consentrecord.DecisionAccepted
}

// alreadyAsAsked is whether the answer before already is what the change asks: given at the
// current version, or not given.
func alreadyAsAsked(before []consentrecords.Record, accepted bool) bool {
	given := len(before) == 1 && before[0].Decision == consentrecord.DecisionAccepted
	if accepted {
		return given && before[0].Version == news.Label
	}
	return !given
}
