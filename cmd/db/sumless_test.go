package main

import (
	"os"
	"path/filepath"
	"testing"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"
	"github.com/stretchr/testify/require"
)

func TestSumlessDirValidatesWithoutAnIntegrityFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "20260101000000_a.sql"),
		[]byte("-- +goose Up\nSELECT 1;\n\n-- +goose Down\nSELECT 2;\n"), 0o644))
	goose, err := sqltool.NewGooseDir(root)
	require.NoError(t, err)

	require.ErrorIs(t, migrate.Validate(goose), migrate.ErrChecksumNotFound, "the library insists on the file")

	dir := sumlessDir{goose}
	require.NoError(t, migrate.Validate(dir))
	sum, err := dir.Checksum()
	require.NoError(t, err)
	require.NoError(t, migrate.WriteSumFile(dir, sum))
	require.NoFileExists(t, filepath.Join(root, migrate.HashFileName))
}
