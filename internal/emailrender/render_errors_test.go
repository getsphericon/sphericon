package emailrender_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

func TestRenderEmailNamesTheFailingPart(t *testing.T) {
	_, err := emailrender.RenderEmail("{% if %}broken", sampleMJML, nil)
	assert.ErrorContains(t, err, "render subject")

	_, err = emailrender.RenderEmail("fine", "<mjml><mj-body>{% if %}broken</mj-body></mjml>", nil)
	assert.ErrorContains(t, err, "render body")
}
