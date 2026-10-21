package oauthserver

import (
	"slices"
	"strings"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// The scope vocabulary is declared once, in the contract (typespec/common/scopes.tsp):
// GrantableScope is what an OAuth client is granted by default, SendScope what needs
// the user's explicit opt-in (ADR 0016, "Send is a second lock"). Adding a scope means
// editing TypeSpec and regenerating.

func grantableScopes() []string {
	return lo.Map(siteapi.GrantableScope("").AllValues(), func(s siteapi.GrantableScope, _ int) string { return string(s) })
}

func sendScopes() []string {
	return lo.Map(siteapi.SendScope("").AllValues(), func(s siteapi.SendScope, _ int) string { return string(s) })
}

// SupportedScopes lists every scope the authorization server can issue.
func SupportedScopes() []string {
	return slices.Concat(grantableScopes(), sendScopes())
}

// requestedScopes splits an OAuth scope parameter into the scopes granted by
// default and the send-class scopes awaiting the user's opt-in. Unknown scopes
// are dropped; an empty request means the default (non-send) set.
func requestedScopes(scope string) (granted, send []string) {
	grantable, sendClass := grantableScopes(), sendScopes()
	fields := strings.Fields(scope)
	if len(fields) == 0 {
		return grantable, nil
	}
	for _, s := range fields {
		switch {
		case slices.Contains(grantable, s) && !slices.Contains(granted, s):
			granted = append(granted, s)
		case slices.Contains(sendClass, s) && !slices.Contains(send, s):
			send = append(send, s)
		}
	}
	mcpSend := string(siteapi.SendScopeMcpSend)
	if len(send) > 0 && !slices.Contains(send, mcpSend) {
		send = append(send, mcpSend)
	}
	return granted, send
}
