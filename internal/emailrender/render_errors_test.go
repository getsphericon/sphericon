package emailrender_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

func TestRenderEmailNamesTheFailingPart(t *testing.T) {
	_, err := emailrender.RenderEmail("{% if %}broken", sampleHTML, nil)
	require.ErrorContains(t, err, "render subject")

	_, err = emailrender.RenderEmail("fine", "<p>{% if %}broken</p>", nil)
	assert.ErrorContains(t, err, "render body")
}
