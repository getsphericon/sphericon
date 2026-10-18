package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

func TestVersionPrintsOneLineOnStdout(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, runVersion(&out, nil))

	assert.Equal(t, buildinfo.Current().Line()+"\n", out.String())
	assert.NotContains(t, out.String(), "commit none")
}

func TestVersionJSONPrintsTheSameAsAnObject(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, runVersion(&out, []string{"--json"}))

	var got map[string]string
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, map[string]string{
		"version": buildinfo.Current().Version,
		"commit":  buildinfo.Current().Commit,
		"built":   buildinfo.Current().Date,
	}, got)
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("\n")))
}

func TestVersionRejectsUnknownArguments(t *testing.T) {
	require.Error(t, runVersion(&bytes.Buffer{}, []string{"--yaml"}))
}
