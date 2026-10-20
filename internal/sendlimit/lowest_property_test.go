package sendlimit_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

func optionalCeiling(t *rapid.T, label string) *int {
	if !rapid.Bool().Draw(t, label+" set") {
		return nil
	}
	return new(rapid.IntRange(1, 100_000).Draw(t, label))
}

// The ceiling is the lower of the manual and the provider value, the manual one on a tie, and no
// ceiling when neither is set; its source says which one it is.
func TestEffectiveOfIsTheLowerCeilingAndNamesItsSource(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		manual := optionalCeiling(t, "manual")
		provider := optionalCeiling(t, "provider")

		got := sendlimit.EffectiveOf(&ent.Integration{MaxPerSecond: manual, ProviderMaxPerSecond: provider}).PerSecond

		switch {
		case manual == nil && provider == nil:
			if got.Limit != nil || got.Source != "" {
				t.Fatalf("no ceiling expected, got %v from %q", got.Limit, got.Source)
			}
		case provider == nil || (manual != nil && *manual <= *provider):
			if got.Limit == nil || *got.Limit != *manual || got.Source != sendlimit.SourceManual {
				t.Fatalf("manual %v (provider %v) expected, got %v from %q", *manual, provider, got.Limit, got.Source)
			}
		default:
			if got.Limit == nil || *got.Limit != *provider || got.Source != sendlimit.SourceProvider {
				t.Fatalf("provider %v (manual %v) expected, got %v from %q", *provider, manual, got.Limit, got.Source)
			}
		}
	})
}
