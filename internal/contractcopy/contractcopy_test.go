package contractcopy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/contractcopy"
)

func TestCheckFlagsEveryCopyAndUndeclaredFixtureScope(t *testing.T) {
	findings, err := contractcopy.Check("testdata/bad", "openapi")
	require.NoError(t, err)
	require.Len(t, findings, 8)
	require.Contains(t, findings[0], "a.go:3")
	require.Contains(t, findings[1], "a.go:5")
	require.Contains(t, findings[2], "a.go:9")
	require.Contains(t, findings[3], "a.go:16")
	require.Contains(t, findings[4], "b.ts:1")
	require.Contains(t, findings[5], "b.ts:2")
	require.Contains(t, findings[6], "b.ts:3")
	require.Contains(t, findings[7], "t.yml:1")
	require.Contains(t, findings[7], "contacts:erase")
	require.Regexp(t, `rule 6: .*; fix: generate it from TypeSpec$`, findings[0])
}

func TestCheckPassesDocumentedAndUnrelatedValues(t *testing.T) {
	findings, err := contractcopy.Check("testdata/clean", "openapi")
	require.NoError(t, err)
	require.Empty(t, findings)
}
