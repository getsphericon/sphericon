package changelog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sample = `# Changelog

## [1.0.0](https://github.com/o/r/compare/v0.2.0...v1.0.0) (2026-11-01)

### ⚠ BREAKING CHANGES

* **segments:** remove Segment type
Upgrade: run the segment export first,
then install 1.0.0.

### Features

- **api:** a thing ([abc](https://example.com))

## [0.2.0](https://github.com/o/r/compare/v0.1.0...v0.2.0) (2026-10-20)

### ⚠ BREAKING CHANGES

* **config:** Upgrade: set SPHERICON_FOO before the restart

### Features

- **api:** another thing
- Upgrade: not a release note outside the breaking section

## [0.1.10](https://github.com/o/r/compare/v0.1.9...v0.1.10) (2026-10-12)

### Bug Fixes

- **x:** nothing to do

## 0.1.0 (2026-10-10)

### ⚠ BREAKING CHANGES

- **x:** **Upgrade:** first release note
`

func TestParseReadsEveryReleaseNewestFirstWithItsUpgradeEntries(t *testing.T) {
	got := Parse(sample)

	require.Len(t, got, 4)
	assert.Equal(t, "1.0.0", got[0].Version)
	assert.Equal(t, []string{"run the segment export first, then install 1.0.0."}, got[0].Upgrades)
	assert.Equal(t, "0.2.0", got[1].Version)
	assert.Equal(t, []string{"set SPHERICON_FOO before the restart"}, got[1].Upgrades)
	assert.Equal(t, "0.1.10", got[2].Version)
	assert.Empty(t, got[2].Upgrades)
	assert.Equal(t, "0.1.0", got[3].Version)
	assert.Equal(t, []string{"first release note"}, got[3].Upgrades)
}

func TestSinceKeepsOnlyReleasesAfterTheVersionThatHaveNotes(t *testing.T) {
	got, err := Since(Parse(sample), "v0.1.9")

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "1.0.0", got[0].Version)
	assert.Equal(t, "0.2.0", got[1].Version)
}

func TestSinceTheNewestVersionIsEmpty(t *testing.T) {
	got, err := Since(Parse(sample), "1.0.0")

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSinceRejectsAnInvalidVersion(t *testing.T) {
	_, err := Since(Parse(sample), "latest")

	assert.Error(t, err)
}

func TestRender(t *testing.T) {
	got, err := Since(Parse(sample), "0.1.10")
	require.NoError(t, err)

	assert.Equal(t, "1.0.0\n  - run the segment export first, then install 1.0.0.\n\n0.2.0\n  - set SPHERICON_FOO before the restart\n", Render(got))
}

func TestRenderNothing(t *testing.T) {
	assert.Empty(t, Render(nil))
}
