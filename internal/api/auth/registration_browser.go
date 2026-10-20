package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

// RegistrationCookie is the name of the cookie that makes a browser the Registration
// browser (ADR 0030): registration sets it, and the verification link trusts the
// password from the form only when it comes back with it.
const RegistrationCookie = "REG"

// registrationCookiePath is the auth prefix shared by the two operations that read the
// cookie, verify and resend; a narrower path would hide it from one of them. The cookie
// is httpOnly and SameSite=Strict, so the browser still sends it to nothing else.
const registrationCookiePath = "/site/auth"

// The claims a verification token carries beyond its User: the address it was sent to
// and the hash of the Registration browser it was minted for.
const (
	claimEmail   = "email"
	claimBrowser = "registration_browser"
)

// VerifyClaims builds the claims of a verification token: the address it is sent to and
// the hash of the registration cookie it trusts (empty for none).
func VerifyClaims(email, browserHash string) map[string]string {
	return map[string]string{claimEmail: email, claimBrowser: browserHash}
}

// VerifyClaimsEmail is the address a verification token was sent to.
func VerifyClaimsEmail(claims map[string]string) string { return claims[claimEmail] }

// VerifyClaimsBrowser is the hash of the Registration browser a verification token was
// minted for, empty when none.
func VerifyClaimsBrowser(claims map[string]string) string { return claims[claimBrowser] }

// NewRegistrationBrowser draws the random value that marks one browser, returning the
// cookie that carries it (httpOnly, SameSite=Strict, scoped to the auth prefix,
// living for ttl) and the hash that goes into the verification token. The value itself
// never leaves the browser.
func (s *Sessions) NewRegistrationBrowser(ttl time.Duration) (cookie *http.Cookie, hash string) {
	value := rand.Text()
	return &http.Cookie{
		Name: RegistrationCookie, Value: value, Path: registrationCookiePath, MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	}, RegistrationHash(value)
}

// RegistrationHash is the hash of a registration cookie's value, empty for no cookie.
func RegistrationHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// SameRegistrationBrowser reports whether a request's registration cookie value is the
// one whose hash a verification token carries. A token minted without a hash (a link
// asked for from a browser that holds no cookie) matches no browser.
func SameRegistrationBrowser(tokenHash, cookieValue string) bool {
	if tokenHash == "" || cookieValue == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHash), []byte(RegistrationHash(cookieValue))) == 1
}
