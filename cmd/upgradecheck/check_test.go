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

func TestCheckBreakingNeedsAnUpgradeFooter(t *testing.T) {
	for _, tc := range []struct {
		name, title, body string
		commits           []string
		wantErr           bool
	}{
		{"nothing breaking", "feat: a", "", []string{"feat: a"}, false},
		{"breaking title without a footer", "feat!: a", "", []string{"feat!: a"}, true},
		{"footer in a commit", "feat!: a", "", []string{"feat!: a\n\nUpgrade: run sphericon migrate"}, false},
		{"footer in the pull request body", "feat!: a", "Notes\n\nUpgrade: rename the env var", []string{"feat!: a"}, false},
		{"breaking commit under a plain title", "feat: a", "", []string{"feat!: a", "fix: b"}, true},
		{"breaking commit footer satisfied by another commit", "feat: a", "", []string{"feat: a\n\nBREAKING CHANGE: x", "docs: b\n\nUpgrade: do y"}, false},
		{"empty footer does not count", "feat!: a", "Upgrade:   ", []string{"feat!: a"}, true},
		{"footer must start a line", "feat!: a", "see Upgrade: later", []string{"feat!: a"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckBreaking(tc.title, tc.body, tc.commits)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "Upgrade:")
			} else {
				require.NoError(t, err)
			}
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

func TestCheckMigrations(t *testing.T) {
	finding := []Finding{{File: "a.sql", Code: "DS102", Text: "Dropping table"}}
	for _, tc := range []struct {
		name, title string
		findings    []Finding
		wantErr     bool
	}{
		{"clean", "feat: a", nil, false},
		{"destructive under a plain title", "feat: a", finding, true},
		{"destructive under a breaking title", "feat!: a", finding, false},
		{"clean under a breaking title", "feat!: a", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckMigrations(tc.title, tc.findings)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "a.sql")
			} else {
				require.NoError(t, err)
			}
		})
	}
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
