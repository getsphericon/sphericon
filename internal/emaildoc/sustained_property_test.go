package emaildoc_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// Whatever the polls, the guard fires exactly when the last `need` of them all read over the
// limit: a model of the run is the whole specification.
func TestSustainedFiresOnlyAfterAnUnbrokenRunOverTheLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		need := rapid.IntRange(1, 12).Draw(t, "need")
		polls := rapid.SliceOfN(rapid.Bool(), 0, 80).Draw(t, "polls")
		s := emaildoc.NewSustained(need)

		run := 0
		for i, over := range polls {
			if over {
				run++
			} else {
				run = 0
			}
			want := run >= need
			if got := s.Observe(over); got != want {
				t.Fatalf("poll %d (run of %d over, need %d): fired %v, want %v", i, run, need, got, want)
			}
		}
	})
}
