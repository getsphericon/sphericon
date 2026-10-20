package auth

import (
	"net/http"
	"time"
)

// SigninFlowCookie is the name of the cookie that carries a provider sign-in's flow
// between `start` and `callback` (ADR 0048).
const SigninFlowCookie = "SIGNIN"

// signinFlowPath is the prefix of the two operations that use the cookie, start and
// callback; the browser sends it to nothing else.
const signinFlowPath = "/site/auth/signin"

// SigninFlow wraps the sealed flow in its cookie: httpOnly, living for ttl, and SameSite=Lax
// (not Strict like the Registration browser's): the provider sends the browser back with a
// cross-site top-level redirect, which Strict would withhold the cookie from.
func (s *Sessions) SigninFlow(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name: SigninFlowCookie, Value: value, Path: signinFlowPath, MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	}
}

// ClearedSigninFlow is the cookie that removes the flow from the browser.
func (s *Sessions) ClearedSigninFlow() *http.Cookie {
	c := s.SigninFlow("", -1)
	c.Expires = time.Unix(0, 0)
	return c
}
