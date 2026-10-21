package server

import "strings"

// secretPathPrefixes are the routes that carry a credential in the path: the signed
// tracking, unsubscribe and confirm tokens and the ingest hook key.
var secretPathPrefixes = []string{"/e/o/", "/e/c/", "/e/u/", "/e/confirm/", "/hooks/"}

// loggedPath is the request path as it may appear in a log line: the credential after a
// secret prefix is replaced, the rest is kept.
func loggedPath(path string) string {
	for _, prefix := range secretPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return prefix + "{redacted}"
		}
	}
	return path
}
