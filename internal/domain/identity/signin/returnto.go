package signin

import (
	"net/url"
	"strings"
)

// SafeReturnTo is the `return_to` of a sign-in as far as it may be honoured: a relative
// in-app path ("/w/abc/contacts?tab=1"), or "" for anything else. Absolute and
// protocol-relative URLs ("https://evil.test", "//evil.test"), backslashes (a browser
// reads them as slashes) and control characters all fall back to "", so the flow cannot
// send a person anywhere but back into the app.
func SafeReturnTo(raw string) string {
	if raw == "" || raw[0] != '/' || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, `\`) {
		return ""
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	return raw
}
