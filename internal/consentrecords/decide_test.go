package consentrecords_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	terms   = signupitems.Document{VersionID: 11, Key: "terms", Title: "Terms of service", Kind: signupitem.KindRequiredAcceptance, Label: "v2"}
	privacy = signupitems.Document{VersionID: 12, Key: "privacy", Title: "Privacy", Kind: signupitem.KindRequiredAcceptance, Label: "v1"}
	news    = signupitems.Document{VersionID: 21, Key: "news", Title: "News", Kind: signupitem.KindOptionalConsent, Label: "v1"}
	notice  = signupitems.Document{VersionID: 31, Key: "notice", Title: "Notice", Kind: signupitem.KindNotice, Label: "v1"}
	items   = []signupitems.Document{terms, privacy, news, notice}
)

// refused is the fields a refused decision names; the decision must have been refused.
func refused(t *testing.T, err error) []string {
	t.Helper()
	fields, ok := errors.AsType[*validation.FieldsError](err)
	require.Truef(t, ok, "refused, got %v", err)
	return slices.Sorted(maps.Keys(fields.Fields))
}

func TestEveryAnsweredItemIsRecordedAndEveryNoticeShown(t *testing.T) {
	entries, err := consentrecords.Decide(items, []consentrecords.Answer{
		{Key: "terms", Version: "v2", Accepted: true},
		{Key: "privacy", Version: "v1", Accepted: true},
		{Key: "news", Version: "v1", Accepted: false},
	})

	require.NoError(t, err)
	assert.Equal(t, []consentrecords.Entry{
		{VersionID: terms.VersionID, Decision: consentrecord.DecisionAccepted},
		{VersionID: privacy.VersionID, Decision: consentrecord.DecisionAccepted},
		{VersionID: news.VersionID, Decision: consentrecord.DecisionDeclined},
		{VersionID: notice.VersionID, Decision: consentrecord.DecisionShown},
	}, entries)
}

func TestAnOptionalConsentLeftOutIsNotRecorded(t *testing.T) {
	entries, err := consentrecords.Decide(items, []consentrecords.Answer{
		{Key: "terms", Version: "v2", Accepted: true},
		{Key: "privacy", Version: "v1", Accepted: true},
	})

	require.NoError(t, err)
	recorded := lo.Map(entries, func(e consentrecords.Entry, _ int) int64 { return e.VersionID })
	assert.Equal(t, []int64{terms.VersionID, privacy.VersionID, notice.VersionID}, recorded)
}

func TestEveryRequiredAcceptanceLeftUntickedIsNamed(t *testing.T) {
	_, err := consentrecords.Decide(items, []consentrecords.Answer{
		{Key: "terms", Version: "v2", Accepted: false},
		{Key: "news", Version: "v1", Accepted: true},
	})

	assert.Equal(t, []string{"signupItems.privacy", "signupItems.terms"}, refused(t, err))
}

func TestAnUnknownItemAStaleVersionOrATwiceNamedItemRefusesTheForm(t *testing.T) {
	valid := []consentrecords.Answer{{Key: "terms", Version: "v2", Accepted: true}, {Key: "privacy", Version: "v1", Accepted: true}}
	cases := map[string]consentrecords.Answer{
		"unknown item":  {Key: "gone", Version: "v1", Accepted: true},
		"stale version": {Key: "news", Version: "v0", Accepted: true},
		"named twice":   {Key: "terms", Version: "v2", Accepted: true},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := consentrecords.Decide(items, append(slices.Clone(valid), extra))

			assert.Equal(t, []string{"signupItems"}, refused(t, err))
		})
	}
}

func TestWithNoItemsInEffectNothingIsAskedOrRecorded(t *testing.T) {
	entries, err := consentrecords.Decide(nil, nil)

	require.NoError(t, err)
	assert.Empty(t, entries)
}

// genItems draws the items in effect: distinct keys, each kind, one of two labels.
func genItems(t *rapid.T) []signupitems.Document {
	keys := rapid.SliceOfDistinct(rapid.SampledFrom([]string{"a", "b", "c", "d"}), rapid.ID).Draw(t, "keys")
	kinds := []signupitem.Kind{signupitem.KindRequiredAcceptance, signupitem.KindOptionalConsent, signupitem.KindNotice}
	docs := make([]signupitems.Document, len(keys))
	for i, key := range keys {
		docs[i] = signupitems.Document{
			VersionID: int64(i + 1), Key: key, Title: key,
			Kind:  rapid.SampledFrom(kinds).Draw(t, "kind"),
			Label: rapid.SampledFrom([]string{"v1", "v2"}).Draw(t, "label"),
		}
	}
	return docs
}

// genAnswers draws answers that may name items not in effect, other labels and an item twice.
func genAnswers(t *rapid.T) []consentrecords.Answer {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) consentrecords.Answer {
		return consentrecords.Answer{
			Key:      rapid.SampledFrom([]string{"a", "b", "c", "d", "e"}).Draw(t, "key"),
			Version:  rapid.SampledFrom([]string{"v1", "v2"}).Draw(t, "version"),
			Accepted: rapid.Bool().Draw(t, "accepted"),
		}
	}), 0, 6).Draw(t, "answers")
}

// genTakenAnswers draws a form that is taken: each required acceptance ticked at its current
// version, any other item answered or not, ticked or not.
func genTakenAnswers(t *rapid.T, docs []signupitems.Document) []consentrecords.Answer {
	var answers []consentrecords.Answer
	for _, d := range docs {
		required := d.Kind == signupitem.KindRequiredAcceptance
		if required || rapid.Bool().Draw(t, "answered") {
			answers = append(answers, consentrecords.Answer{Key: d.Key, Version: d.Label, Accepted: required || rapid.Bool().Draw(t, "accepted")})
		}
	}
	return answers
}

// wellFormed reports whether every answer names an item in effect, once, at its current version.
func wellFormed(docs []signupitems.Document, answers []consentrecords.Answer) bool {
	byKey := lo.KeyBy(docs, func(d signupitems.Document) string { return d.Key })
	keys := lo.Map(answers, func(a consentrecords.Answer, _ int) string { return a.Key })
	current := lo.EveryBy(answers, func(a consentrecords.Answer) bool { return byKey[a.Key].Label == a.Version && byKey[a.Key].Key != "" })
	return current && len(lo.Uniq(keys)) == len(keys)
}

// allAccepted reports whether every required acceptance is answered ticked.
func allAccepted(docs []signupitems.Document, answers []consentrecords.Answer) bool {
	required := lo.Filter(docs, func(d signupitems.Document, _ int) bool { return d.Kind == signupitem.KindRequiredAcceptance })
	return lo.EveryBy(required, func(d signupitems.Document) bool {
		return lo.ContainsBy(answers, func(a consentrecords.Answer) bool { return a.Key == d.Key && a.Accepted })
	})
}

// A form is taken exactly when every answer names an item in effect once at its current version
// and every required acceptance is ticked; a refusal records nothing.
func TestAFormIsTakenExactlyWhenWellFormedAndEveryRequiredAcceptanceIsTicked(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		docs := genItems(t)
		answers := genAnswers(t)

		entries, err := consentrecords.Decide(docs, answers)

		want := wellFormed(docs, answers) && allAccepted(docs, answers)
		assert.Equal(t, want, err == nil, "taken")
		assert.True(t, err == nil || len(entries) == 0, "a refusal records nothing")
	})
}

// A taken form records each required acceptance accepted, each notice shown, and nothing twice or
// outside the items in effect.
func TestATakenFormRecordsEveryRequiredAcceptanceAndNoticeOnceAtItsCurrentVersion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		docs := genItems(t)
		answers := genTakenAnswers(t, docs)

		entries, err := consentrecords.Decide(docs, answers)

		require.NoError(t, err)
		byVersion := lo.KeyBy(entries, func(e consentrecords.Entry) int64 { return e.VersionID })
		assert.Len(t, byVersion, len(entries), "no version twice")
		inEffect := lo.KeyBy(docs, func(d signupitems.Document) int64 { return d.VersionID })
		assert.True(t, lo.EveryBy(entries, func(e consentrecords.Entry) bool { _, ok := inEffect[e.VersionID]; return ok }))
		required := lo.Filter(docs, func(d signupitems.Document, _ int) bool { return d.Kind == signupitem.KindRequiredAcceptance })
		notices := lo.Filter(docs, func(d signupitems.Document, _ int) bool { return d.Kind == signupitem.KindNotice })
		assert.True(t, lo.EveryBy(required, func(d signupitems.Document) bool {
			return byVersion[d.VersionID].Decision == consentrecord.DecisionAccepted
		}))
		assert.True(t, lo.EveryBy(notices, func(d signupitems.Document) bool {
			return byVersion[d.VersionID].Decision == consentrecord.DecisionShown
		}))
	})
}
