package logging

import "strings"

// secretPathPrefixes are the routes that carry a credential in the path: the signed
// tracking, unsubscribe and confirm tokens, the short link token, the invitation token and the ingest hook key.
// The site ones are not under /site/workspaces/, so a prefix never swallows a scoped path.
var secretPathPrefixes = []string{
	"/e/o/", "/e/c/", "/e/u/", "/e/confirm/", "/c/", "/hooks/",
	"/site/confirmations/", "/site/unsubscribes/", "/site/invitations/",
}

// RedactedPath is the request path as it may appear in a log line: the credential after a
// secret prefix is replaced, the rest is kept.
func RedactedPath(path string) string {
	for _, prefix := range secretPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return prefix + "{redacted}"
		}
	}
	return path
}
