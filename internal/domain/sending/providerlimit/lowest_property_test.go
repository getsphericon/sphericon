package providerlimit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
)

func optionalCeiling(t *rapid.T, label string) *int {
	if !rapid.Bool().Draw(t, label+" set") {
		return nil
	}
	return new(rapid.IntRange(1, 100_000).Draw(t, label))
}

// lowerCeiling is the specification of the ceiling: the lower of the two values, the manual one
// on a tie, and none when neither is set.
func lowerCeiling(manual, provider *int) providerlimit.Value {
	switch {
	case manual == nil && provider == nil:
		return providerlimit.Value{}
	case provider == nil || (manual != nil && *manual <= *provider):
		return providerlimit.Value{Limit: manual, Source: providerlimit.SourceManual}
	default:
		return providerlimit.Value{Limit: provider, Source: providerlimit.SourceProvider}
	}
}

// The ceiling is the lower of the manual and the provider value, the manual one on a tie, and no
// ceiling when neither is set; its source says which one it is.
func TestEffectiveOfIsTheLowerCeilingAndNamesItsSource(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		manual := optionalCeiling(t, "manual")
		provider := optionalCeiling(t, "provider")
		want := lowerCeiling(manual, provider)

		got := providerlimit.EffectiveOf(integ(ent.Integration{MaxPerSecond: manual}, ent.IntegrationProviderState{ProviderMaxPerSecond: provider})).PerSecond

		assert.Equalf(t, want, got, "manual %v, provider %v", manual, provider)
	})
}
