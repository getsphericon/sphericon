package smsrender_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/smsrender"
)

// partsCase is one row of the table both counters read (testdata/parts.json): a text given as
// pieces, each repeated, and the figures worked out by hand.
type partsCase struct {
	Name       string            `json:"name"`
	Text       []json.RawMessage `json:"text"`
	Encoding   string            `json:"encoding"`
	Characters int               `json:"characters"`
	Parts      int               `json:"parts"`
}

// caseText is the text of c, its pieces repeated and joined.
func caseText(t *testing.T, c partsCase) string {
	t.Helper()
	var b strings.Builder
	for _, raw := range c.Text {
		var piece []any
		require.NoError(t, json.Unmarshal(raw, &piece))
		b.WriteString(strings.Repeat(piece[0].(string), int(piece[1].(float64))))
	}
	return b.String()
}

func TestMeasureAgreesWithTheSharedTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/parts.json")
	require.NoError(t, err)
	var table struct {
		Cases []partsCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &table))
	require.NotEmpty(t, table.Cases)

	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := smsrender.Measure(caseText(t, c))

			assert.Equal(t, smsrender.Size{Encoding: smsrender.Encoding(c.Encoding), Characters: c.Characters, Parts: c.Parts}, got)
		})
	}
}

func TestOneCharacterOutsideGSM7MakesAnyTextUCS2(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := []rune(rapid.StringOf(rapid.RuneFrom([]rune("abcXYZ019 .,!?@£$éÇ€{}"))).Draw(t, "text"))
		at := rapid.IntRange(0, len(text)).Draw(t, "at")

		got := smsrender.Measure(string(text[:at]) + "ж" + string(text[at:]))

		assert.Equal(t, smsrender.UCS2, got.Encoding)
	})
}

func TestALongerTextNeverTakesFewerParts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := rapid.String().Draw(t, "text")
		more := rapid.String().Draw(t, "more")

		shorter, longer := smsrender.Measure(text), smsrender.Measure(text+more)

		assert.LessOrEqual(t, shorter.Parts, longer.Parts)
	})
}

func TestCheckPartsRefusesATextOverTheLimit(t *testing.T) {
	at := strings.Repeat("a", 306)

	require.NoError(t, smsrender.CheckParts(at, 2, 0))
	err := smsrender.CheckParts(at+"a", 2, 0)

	tooLong, ok := errors.AsType[*smsrender.TooManyPartsError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, smsrender.TooManyPartsError{Size: smsrender.Size{Encoding: smsrender.GSM7, Characters: 307, Parts: 3}, Limit: 2}, *tooLong)
	assert.Equal(t, "the text is 307 characters in GSM-7, 3 segments; it may take 2 segments at most", err.Error())
}

// linksCase is one row of testdata/links.json: a text, the links found in it, and its figures once
// each link is a short link of linkLength characters.
type linksCase struct {
	partsCase

	Links      []string `json:"links"`
	LinkLength int      `json:"linkLength"`
}

func TestLinksAndTheirShortLengthAgreeWithTheSharedTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/links.json")
	require.NoError(t, err)
	var table struct {
		Cases []linksCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &table))
	require.NotEmpty(t, table.Cases)

	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			text := caseText(t, c.partsCase)

			require.NotNil(t, c.Links, "every case names its links")
			assert.Equal(t, c.Links, smsrender.Links(text))
			assert.Equal(t, smsrender.Size{Encoding: smsrender.Encoding(c.Encoding), Characters: c.Characters, Parts: c.Parts},
				smsrender.MeasureLinked(text, c.LinkLength))
		})
	}
}

func TestRewriteLinksReplacesEveryLinkAndKeepsTheRest(t *testing.T) {
	got, err := smsrender.RewriteLinks("Shop https://a.example/x, then https://b.example/y. Bye", func(url string) (string, error) {
		return "<" + url + ">", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "Shop <https://a.example/x>, then <https://b.example/y>. Bye", got)
}

func TestRewriteLinksStopsAtTheFirstError(t *testing.T) {
	boom := errors.New("boom")

	_, err := smsrender.RewriteLinks("https://a.example/x", func(string) (string, error) { return "", boom })

	assert.ErrorIs(t, err, boom)
}

func TestCheckPartsCountsTheShortLinks(t *testing.T) {
	text := strings.Repeat("a", 280) + " https://shop.example/" + strings.Repeat("p", 100)

	require.Error(t, smsrender.CheckParts(text, 2, 0), "as written the text takes three parts")
	assert.NoError(t, smsrender.CheckParts(text, 2, 25), "with its link short it takes two")
}
