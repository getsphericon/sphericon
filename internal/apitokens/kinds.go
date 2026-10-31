package apitokens

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/accounts"
)

// An api_tokens row is one of three credentials (ADR 0061), told apart here and nowhere else:
// the Workspace's API token has no Membership, a Connection names the OAuth client its User
// approved, and a Personal token is a User's own token that names no client.

// isWorkspaceToken is the Workspace's API tokens.
func isWorkspaceToken() predicate.ApiToken {
	return apitoken.MembershipIDIsNil()
}

// isConnection is the Connections. The schema's check gives every one a Membership.
func isConnection() predicate.ApiToken {
	return apitoken.OauthClientIDNotNil()
}

// isPersonalToken is the Personal tokens.
func isPersonalToken() predicate.ApiToken {
	return apitoken.And(apitoken.MembershipIDNotNil(), apitoken.OauthClientIDIsNil())
}

// visibleTo is the User's credentials (Connections and Personal tokens) caller may see and
// revoke: the caller's own, or every member's when the Role holds members:manage. It is the Policy
// of both, so every surface that lists or revokes them gets it.
func visibleTo(caller *ent.Membership) predicate.ApiToken {
	if accounts.Allows(caller.Role, accounts.PermMembersManage) {
		return apitoken.MembershipIDNotNil()
	}
	return apitoken.MembershipID(caller.ID)
}
