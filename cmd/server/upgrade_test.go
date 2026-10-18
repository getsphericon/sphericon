package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpgradeNeedsASource(t *testing.T) {
	err := runUpgrade(t.Context(), nil, &bytes.Buffer{})

	assert.ErrorContains(t, err, "usage: upgrade --from")
}

func TestUpgradeRejectsUnknownFlags(t *testing.T) {
	err := runUpgrade(t.Context(), []string{"--bogus"}, &bytes.Buffer{})

	assert.ErrorContains(t, err, "usage: upgrade --from")
}
