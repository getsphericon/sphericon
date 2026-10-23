package testhelper

import "net/http"

// RequestCookie is the cookie a test sends with a request: a browser returns only the name and
// the value, so the attributes are the ones the server sets (Secure, HttpOnly, SameSite) and
// matter to nothing but the one place that builds the struct.
func RequestCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	}
}
