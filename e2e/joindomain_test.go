//go:build e2e

package e2e

import "testing"

// An Owner proves a domain and switches domain join on; a colleague signs up on it through
// the real flow and reaches the Workspace from the verify mail alone.
func TestAColleagueSigningUpOnAJoinDomainReachesTheWorkspaceThroughTheVerifyMail(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t)
	domain := w.AddVerifiedJoinDomain()
	w.EnableDomainJoin()

	colleague := w.SignUpColleague(domain)

	w.RequireJoinedByDomain(colleague)
}
