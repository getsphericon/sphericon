package auth

import (
	"net/http"
	"time"
)

// SignUpBrowserCookie is the name of the cookie that makes a browser the Sign-up browser
// (ADR 0030): a Sign-up sets it, and the verification link trusts the password from the form
// only when it comes back with it. Its value is drawn by the signup module.
const SignUpBrowserCookie = "SIGN_UP_BROWSER"

// signUpBrowserCookiePath is the auth prefix shared by the two operations that read the
// cookie, verify and resend; a narrower path would hide it from one of them. The cookie
// is httpOnly and SameSite=Strict, so the browser still sends it to nothing else.
const signUpBrowserCookiePath = "/site/auth"

// SignUpBrowser is the cookie that carries a Sign-up browser's value (httpOnly,
// SameSite=Strict, scoped to the auth prefix, living for ttl). The value itself never leaves
// the browser; the verification token carries only its hash.
func (s *Sessions) SignUpBrowser(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name: SignUpBrowserCookie, Value: value, Path: signUpBrowserCookiePath, MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	}
}
