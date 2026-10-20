package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tc.want, IsBreaking(tc.message))
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
			err := CheckBreaking(tc.title, tc.body, tc.commits)

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
			err := CheckBreaking(tc.title, tc.body, tc.commits)

			require.ErrorContains(t, err, "Upgrade:")
		})
	}
}

const atlasFiles = `[
 {"Name":"20260101000000_river_001.sql","Reports":[{"Text":"destructive changes detected","Diagnostics":[{"Code":"DS102","Text":"Dropping table \"river_x\""}]}]},
 {"Name":"20260102000000_add.sql","Reports":null},
 {"Name":"20260103000000_drop.sql","Reports":[{"Text":"destructive changes detected","Diagnostics":[{"Code":"DS103","Text":"Dropping non-virtual column \"a\""}]}]},
 {"Name":"20260104000000_rename.sql","Reports":[{"Text":"backward incompatible","Diagnostics":[{"Code":"BC102","Text":"Renaming column"}]}]},
 {"Name":"20260105000000_concurrent.sql","Reports":[{"Text":"concurrent index","Diagnostics":[{"Code":"PG101","Text":"Creating index non-concurrently"}]}]}
]`

func TestDestructiveFindingsSkipRiverAndOtherAnalyzers(t *testing.T) {
	got, err := Destructive([]byte(atlasFiles))
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{File: "20260103000000_drop.sql", Code: "DS103", Text: `Dropping non-virtual column "a"`},
		{File: "20260104000000_rename.sql", Code: "BC102", Text: "Renaming column"},
	}, got)
}

func TestDestructiveRejectsGarbage(t *testing.T) {
	_, err := Destructive([]byte("not json"))
	require.Error(t, err)
}

func TestCheckMigrationsAcceptsCleanOnesAndDestructiveOnesUnderABreakingTitle(t *testing.T) {
	finding := []Finding{{File: "a.sql", Code: "DS102", Text: "Dropping table"}}
	for _, tc := range []struct {
		name, title string
		findings    []Finding
	}{
		{"clean", "feat: a", nil},
		{"destructive under a breaking title", "feat!: a", finding},
		{"clean under a breaking title", "feat!: a", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckMigrations(tc.title, tc.findings)

			require.NoError(t, err)
		})
	}
}

func TestCheckMigrationsRefusesADestructiveOneUnderAPlainTitle(t *testing.T) {
	findings := []Finding{{File: "a.sql", Code: "DS102", Text: "Dropping table"}}

	err := CheckMigrations("feat: a", findings)

	require.ErrorContains(t, err, "a.sql")
}

func TestLintWindow(t *testing.T) {
	all := []string{
		"20260101_a.sql", "20260102_river_001.sql", "20260103_b.sql", "20260104_river_002.sql", "20260105_c.sql",
	}
	for _, tc := range []struct {
		name  string
		added []string
		want  int
	}{
		{"nothing added", nil, 0},
		{"only river added", []string{"20260102_river_001.sql"}, 0},
		{"newest", []string{"20260105_c.sql"}, 1},
		{"two with a river file between", []string{"20260103_b.sql", "20260105_c.sql"}, 3},
		{"river files skipped when picking the oldest", []string{"20260102_river_001.sql", "20260105_c.sql"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, LintWindow(all, tc.added))
		})
	}
}
