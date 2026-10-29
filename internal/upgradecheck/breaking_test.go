package upgradecheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgradecheck"
)

func TestIsBreaking(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		want          bool
	}{
		{"plain feat", "feat: add a thing", false},
		{"bang", "feat!: drop a column", true},
		{"scoped bang", "refactor(staff)!: rename the table", true},
		{"bang in the middle of a subject is not a marker", "fix: handle a!: b", false},
		{"footer", "feat: x\n\nbody\n\nBREAKING CHANGE: the column is gone", true},
		{"hyphenated footer", "feat: x\n\nBREAKING-CHANGE: gone", true},
		{"footer text in prose does not count", "feat: x\n\nthis is not a BREAKING CHANGE: really", false},
		{"unconventional subject", "Merge branch main", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, upgradecheck.IsBreaking(tc.message))
		})
	}
}

type breakingCase struct {
	name, title, body string
	commits           []string
}

func TestCheckBreakingAcceptsWhatCarriesAnUpgradeFooter(t *testing.T) {
	for _, tc := range []breakingCase{
		{"nothing breaking", "feat: a", "", []string{"feat: a"}},
		{"Upgrade inside the BREAKING CHANGE footer", "feat!: a", "", []string{"feat!: a\n\nBREAKING CHANGE: the column is gone\nUpgrade: run sphericon migrate"}},
		{"Upgrade on the BREAKING CHANGE line", "feat!: a", "", []string{"feat!: a\n\nBREAKING CHANGE: Upgrade: run sphericon migrate"}},
		{"in the pull request body", "feat!: a", "Notes\n\nBREAKING CHANGE: rename\nUpgrade: rename the env var", []string{"feat!: a"}},
		{"breaking commit satisfied by another commit", "feat: a", "", []string{"feat: a\n\nBREAKING CHANGE: x", "docs: b\n\nBREAKING CHANGE: y\nUpgrade: do y"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := upgradecheck.CheckBreaking(tc.title, tc.body, tc.commits)

			require.NoError(t, err)
		})
	}
}

func TestCheckBreakingRefusesABreakingChangeWithoutAnUpgradeFooter(t *testing.T) {
	for _, tc := range []breakingCase{
		{"breaking title without a footer", "feat!: a", "", []string{"feat!: a"}},
		{"bare Upgrade footer is not rendered by release-please", "feat!: a", "", []string{"feat!: a\n\nUpgrade: run sphericon migrate"}},
		{"Upgrade before the BREAKING CHANGE footer does not count", "feat!: a", "Upgrade: x\n\nBREAKING CHANGE: rename", []string{"feat!: a"}},
		{"breaking commit under a plain title", "feat: a", "", []string{"feat!: a", "fix: b"}},
		{"empty Upgrade does not count", "feat!: a", "BREAKING CHANGE: x\nUpgrade:   ", []string{"feat!: a"}},
		{"Upgrade must start a line", "feat!: a", "BREAKING CHANGE: x\nsee Upgrade: later", []string{"feat!: a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := upgradecheck.CheckBreaking(tc.title, tc.body, tc.commits)

			require.ErrorContains(t, err, "Upgrade:")
		})
	}
}
