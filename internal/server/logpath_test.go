package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

func TestLoggedPathNeverKeepsACredential(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		prefix := rapid.SampledFrom(secretPathPrefixes).Draw(t, "prefix")
		credential := rapid.StringMatching(`[A-Za-z0-9._-]{8,64}(/accept)?`).Draw(t, "credential")

		logged := loggedPath(prefix + credential)

		assert.Equal(t, prefix+"{redacted}", logged)
	})
}

func TestLoggedPathKeepsEveryOtherPath(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		path := "/" + rapid.StringMatching(`[a-z0-9/_-]{0,40}`).Draw(t, "path")
		secret := false
		for _, prefix := range secretPathPrefixes {
			secret = secret || strings.HasPrefix(path, prefix)
		}
		if secret {
			t.Skip("a credential route")
		}

		logged := loggedPath(path)

		assert.Equal(t, path, logged)
	})
}

func TestLoggedPathLeavesWorkspaceInvitationsAlone(t *testing.T) {
	logged := loggedPath("/site/workspaces/AbCd1234/invitations/42")

	assert.Equal(t, "/site/workspaces/AbCd1234/invitations/42", logged)
}
