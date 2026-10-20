package exportstream_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/exportstream"
)

func cursorGen() *rapid.Generator[exportstream.Cursor] {
	return rapid.Custom(func(t *rapid.T) exportstream.Cursor {
		micros := rapid.Int64Range(0, 4_000_000_000_000_000).Draw(t, "micros")
		id := rapid.Int64Range(1, 1<<62).Draw(t, "id")
		return exportstream.Cursor{At: time.UnixMicro(micros), ID: id}
	})
}

// A token read back is the position it was issued for, or a consumer would resume elsewhere.
func TestCursorRoundTrips(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := cursorGen().Draw(t, "cursor")

		got, err := exportstream.ParseCursor(c.String())

		if err != nil || !got.At.Equal(c.At) || got.ID != c.ID {
			t.Fatalf("%v came back as %v, %v", c, got, err)
		}
	})
}

// The cursor after a row sorts strictly after it and before anything later, so a page
// neither repeats its last row nor skips one that shares its instant.
func TestFollowingIsTheNextPositionOfTheSameInstant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := cursorGen().Draw(t, "cursor")

		next := c.Following()

		if !next.At.Equal(c.At) || next.ID != c.ID+1 {
			t.Fatalf("%v is followed by %v", c, next)
		}
	})
}

func TestLimitIsBounded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		requested := rapid.Int32().Draw(t, "requested")
		set := rapid.Bool().Draw(t, "set")

		got := exportstream.Limit(requested, set)

		if got < 1 || got > exportstream.MaxLimit {
			t.Fatalf("Limit(%d, %v) = %d", requested, set, got)
		}
		if !set && got != exportstream.DefaultLimit {
			t.Fatalf("an absent limit is %d, not the default", got)
		}
	})
}

// Whatever arrives as a cursor is either a position or the one refusal, never a panic.
func FuzzParseCursor(f *testing.F) {
	f.Add("")
	f.Add(exportstream.Cursor{At: time.UnixMicro(1_700_000_000_000_000), ID: 42}.String())
	f.Add("not-a-cursor")
	f.Add("MTox")
	f.Fuzz(func(t *testing.T, token string) {
		c, err := exportstream.ParseCursor(token)
		refusedCleanly := err == nil || errors.Is(err, exportstream.ErrBadCursor)
		require.Truef(t, refusedCleanly, "%q: unexpected error %v", token, err)
		noPositionWithAnError := err == nil || c.IsZero()
		require.Truef(t, noPositionWithAnError, "%q: a position and an error together", token)
	})
}

// The row check is what keeps a stream's encoder and the published contract one thing.
func TestValidateRowRefusesADriftedRow(t *testing.T) {
	const ok = `{"id":"1","action":"a","properties":{},"contact_id":null,"visitor_id":null,"subject_id":"",` +
		`"email":null,"phone":null,"occurred_at":null,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	err := exportstream.ValidateRow(exportstream.EventsSchema, []byte(ok))
	require.NoErrorf(t, err, "a conforming row is refused: %v", err)
	drifted := map[string]string{
		"an undeclared key":   `{"extra":1,` + ok[1:],
		"a missing key":       `{"id":"1"}`,
		"a wrongly typed key": `{"id":"1","action":7,` + ok[len(`{"id":"1","action":"a",`):],
	}
	for name, row := range drifted {
		err := exportstream.ValidateRow(exportstream.EventsSchema, []byte(row))
		assert.Errorf(t, err, "%s passes", name)
	}
}
