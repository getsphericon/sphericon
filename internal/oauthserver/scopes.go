package oauthserver

import (
	"slices"
	"strings"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
)

// The scope vocabulary is declared once, in the contract (typespec/common/scopes.tsp):
// GrantableScope is what an OAuth client is granted by default, SendScope and SetupScope
// what need the user's explicit opt-in, each with a box of its own (ADR 0016, "Send is a
// second lock"). Adding a scope means editing TypeSpec and regenerating.

func grantableScopes() []string {
	return lo.Map(siteapi.GrantableScope("").AllValues(), func(s siteapi.GrantableScope, _ int) string { return string(s) })
}

func sendScopes() []string {
	return lo.Map(siteapi.SendScope("").AllValues(), func(s siteapi.SendScope, _ int) string { return string(s) })
}

func setupScopes() []string {
	return lo.Map(siteapi.SetupScope("").AllValues(), func(s siteapi.SetupScope, _ int) string { return string(s) })
}

// SupportedScopes lists every scope the authorization server can issue.
func SupportedScopes() []string {
	return slices.Concat(grantableScopes(), sendScopes(), setupScopes())
}

// requestedScopes splits an OAuth scope parameter into the scopes granted by default,
// the send-class scopes and the setup scopes, each of the last two awaiting its own
// opt-in. Unknown scopes are dropped; an empty request means the default set (neither
// send nor setup).
func requestedScopes(scope string) (granted, send, setup []string) {
	grantable, sendClass, setupClass := grantableScopes(), sendScopes(), setupScopes()
	fields := strings.Fields(scope)
	if len(fields) == 0 {
		return grantable, nil, nil
	}
	for _, s := range fields {
		switch {
		case slices.Contains(grantable, s) && !slices.Contains(granted, s):
			granted = append(granted, s)
		case slices.Contains(sendClass, s) && !slices.Contains(send, s):
			send = append(send, s)
		case slices.Contains(setupClass, s) && !slices.Contains(setup, s):
			setup = append(setup, s)
		}
	}
	// Only the send class brings mcp:send: a setup scope is not listed behind it.
	mcpSend := string(siteapi.SendScopeMcpSend)
	if len(send) > 0 && !slices.Contains(send, mcpSend) {
		send = append(send, mcpSend)
	}
	return granted, send, setup
}

// requiredPermissions is the Permission granting each of scopes needs, for those that need one
// (accounts.GrantPermissions, declared on each grantable scope in TypeSpec).
func requiredPermissions(scopes []string) map[string]accounts.Permission {
	needed := map[string]accounts.Permission{}
	for _, scope := range scopes {
		if p, ok := accounts.GrantPermissions[scope]; ok {
			needed[scope] = p
		}
	}
	return needed
}
