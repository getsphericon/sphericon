package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoggedPathRedactsCredentials(t *testing.T) {
	assert.Equal(t, "/e/u/{redacted}", loggedPath("/e/u/eyJhbGciOi.payload.sig"))
	assert.Equal(t, "/e/c/{redacted}", loggedPath("/e/c/token"))
	assert.Equal(t, "/hooks/{redacted}", loggedPath("/hooks/ingest-key"))
	assert.Equal(t, "/site/workspaces", loggedPath("/site/workspaces"))
}
