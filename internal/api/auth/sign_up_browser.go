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

// The claims a verification token carries beyond its User: the address it was sent to
// and the hash of the Sign-up browser it was minted for.
const (
	claimEmail   = "email"
	claimBrowser = "sign_up_browser"
)

// VerifyClaims builds the claims of a verification token: the address it is sent to and
// the hash of the Sign-up browser cookie it trusts (empty for none).
func VerifyClaims(email, browserHash string) map[string]string {
	return map[string]string{claimEmail: email, claimBrowser: browserHash}
}

// VerifyClaimsEmail is the address a verification token was sent to.
func VerifyClaimsEmail(claims map[string]string) string { return claims[claimEmail] }

// VerifyClaimsBrowser is the hash of the Sign-up browser a verification token was minted
// for, empty when none.
func VerifyClaimsBrowser(claims map[string]string) string { return claims[claimBrowser] }

// SignUpBrowser is the cookie that carries a Sign-up browser's value (httpOnly,
// SameSite=Strict, scoped to the auth prefix, living for ttl). The value itself never leaves
// the browser; the verification token carries only its hash.
func (s *Sessions) SignUpBrowser(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name: SignUpBrowserCookie, Value: value, Path: signUpBrowserCookiePath, MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	}
}
