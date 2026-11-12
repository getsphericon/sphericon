package contractcopy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/contractcopy"
)

func TestCheckFlagsEveryCopyAndUndeclaredFixtureScope(t *testing.T) {
	findings, err := contractcopy.Check("testdata/bad", "openapi")
	require.NoError(t, err)
	require.Len(t, findings, 13)
	require.Contains(t, findings[0], "a.go:3")
	require.Contains(t, findings[1], "a.go:7")
	require.Contains(t, findings[2], "b.ts:1")
	require.Contains(t, findings[3], "b.ts:2")
	require.Contains(t, findings[4], "b.ts:3")
	require.Contains(t, findings[5], "c.ts:1")
	require.Contains(t, findings[5], "repeat a TypeSpec enum")
	require.Contains(t, findings[6], "c.ts:6")
	require.Contains(t, findings[6], "the fields of Button")
	require.Contains(t, findings[7], "c.ts:10")
	require.Contains(t, findings[7], "the fields of Button")
	require.Contains(t, findings[8], "c.ts:11")
	require.Contains(t, findings[8], "pageSize = 250")
	require.Contains(t, findings[9], "c.ts:12")
	require.Contains(t, findings[9], "pageSize = 250")
	require.Contains(t, findings[10], "d.ts:1")
	require.Contains(t, findings[10], "string constants repeat a TypeSpec enum")
	require.Contains(t, findings[11], "d.ts:4")
	require.Contains(t, findings[11], "repeats a TypeSpec enum")
	require.Contains(t, findings[12], "t.yml:1")
	require.Contains(t, findings[12], "contacts:erase")
	require.Regexp(t, `rule 6: .*; fix: generate it from TypeSpec$`, findings[0])
}

// A Go bound is not read: the contract's bounds are generated into Go, and the owners use them.
func TestCheckPassesUnrelatedValuesAndGoBounds(t *testing.T) {
	findings, err := contractcopy.Check("testdata/clean", "openapi")
	require.NoError(t, err)
	require.Empty(t, findings)
}
