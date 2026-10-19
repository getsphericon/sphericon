package oauthserver

import (
	"slices"
	"strings"
)

// grantableScopes are the API token scopes an OAuth client may be granted
// without any extra opt-in: the external API's read/write vocabulary. Token
// management scopes are deliberately absent, so a connector can never mint
// further credentials.
var grantableScopes = []string{
	"contacts:read",
	"contacts:write",
	"events:read",
	"events:write",
	"segments:read",
	"segments:write",
	"broadcasts:read",
	"broadcasts:write",
	"automations:read",
	"automations:write",
	"enrollments:read",
	"templates:read",
	"templates:write",
	"webhooks:read",
	"webhooks:write",
	"webhooks:regenerate",
	"webhooks:test",
	"custom_fields:read",
	"sending_domains:read",
	"integrations:read",
}

// sendScopes are the send-class scopes (ADR 0016, "Send is a second lock"). The
// agent authors by default: these are granted only when the user explicitly opts
// in on the consent screen. mcp:send is the MCP-specific key: send-class tools are
// not even listed over /mcp without it, so any send-class grant brings it along.
var sendScopes = []string{
	"emails:send",
	"broadcasts:send",
	"automations:activate",
	scopeMCPSend,
}

const scopeMCPSend = "mcp:send"

// SupportedScopes lists every scope the authorization server can issue.
func SupportedScopes() []string {
	return slices.Concat(grantableScopes, sendScopes)
}

// requestedScopes splits an OAuth scope parameter into the scopes granted by
// default and the send-class scopes awaiting the user's opt-in. Unknown scopes
// are dropped; an empty request means the default (non-send) set.
func requestedScopes(scope string) (granted, send []string) {
	fields := strings.Fields(scope)
	if len(fields) == 0 {
		return slices.Clone(grantableScopes), nil
	}
	for _, s := range fields {
		switch {
		case slices.Contains(grantableScopes, s) && !slices.Contains(granted, s):
			granted = append(granted, s)
		case slices.Contains(sendScopes, s) && !slices.Contains(send, s):
			send = append(send, s)
		}
	}
	if len(send) > 0 && !slices.Contains(send, scopeMCPSend) {
		send = append(send, scopeMCPSend)
	}
	return granted, send
}
