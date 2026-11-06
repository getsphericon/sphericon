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

	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
)

// partsCase is one row of the table both counters read (testdata/parts.json): a text given as
// pieces, each repeated, and the figures worked out by hand.
type partsCase struct {
	Name       string            `json:"name"`
	Text       []json.RawMessage `json:"text"`
	Encoding   string            `json:"encoding"`
	Characters int               `json:"characters"`
	Parts      int               `json:"parts"`
	// Fits is, for a limit case, whether the text is within Twilio's length limit.
	Fits bool `json:"fits"`
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

func TestCheckLengthRefusesATextOverTheLimitOfItsEncoding(t *testing.T) {
	limit := smsrender.Limit{GSM7: 306, UCS2: 70}
	at := strings.Repeat("a", 306)

	fits := smsrender.CheckLength(at, limit, 0)
	err := smsrender.CheckLength(at+"a", limit, 0)

	require.NoError(t, fits)
	tooLong, ok := errors.AsType[*smsrender.TooLongError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, smsrender.TooLongError{Size: smsrender.Size{Encoding: smsrender.GSM7, Characters: 307, Parts: 3}, Limit: 306}, *tooLong)
	assert.Equal(t, "the text is 307 characters in GSM-7 (3 segments); it may take 306 characters in GSM-7 at most", err.Error())
}

// The limit the table pins is Twilio's, and the save refuses exactly the texts the table says do not
// fit it; the editor's counter flags the same ones (src/utils/smsParts.test.ts).
func TestTwiliosLengthLimitAgreesWithTheSharedTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/parts.json")
	require.NoError(t, err)
	var table struct {
		Limit struct {
			GSM7 int `json:"gsm7"`
			UCS2 int `json:"ucs2"`
		} `json:"limit"`
		LimitCases []partsCase `json:"limitCases"`
	}
	require.NoError(t, json.Unmarshal(raw, &table))
	require.NotEmpty(t, table.LimitCases)

	assert.Equal(t, smsrender.Limit{GSM7: table.Limit.GSM7, UCS2: table.Limit.UCS2}, twilio.MaxLength)
	for _, c := range table.LimitCases {
		t.Run(c.Name, func(t *testing.T) {
			err := smsrender.CheckLength(caseText(t, c), twilio.MaxLength, 0)

			assert.Equal(t, c.Fits, err == nil, "refused: %v", err)
		})
	}
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

func TestCheckLengthCountsTheShortLinks(t *testing.T) {
	text := strings.Repeat("a", 280) + " https://shop.example/" + strings.Repeat("p", 100)
	limit := smsrender.Limit{GSM7: 306, UCS2: 134}

	asWritten := smsrender.CheckLength(text, limit, 0)
	shortened := smsrender.CheckLength(text, limit, 25)

	require.Error(t, asWritten, "as written the text is 402 characters")
	assert.NoError(t, shortened, "with its link short it is 306")
}
