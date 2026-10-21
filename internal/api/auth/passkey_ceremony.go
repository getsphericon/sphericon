package auth

import (
	"net/http"
	"time"
)

// PasskeyCeremonyCookie is the name of the cookie that carries a Passkey ceremony's challenge
// between `begin` and `finish` (ADR 0052).
const PasskeyCeremonyCookie = "PASSKEY"

// PasskeyRegistrationPath is the prefix of the registration operations that use the cookie;
// the browser sends it to nothing else.
const PasskeyRegistrationPath = "/site/me/passkeys/registration"

// PasskeyCheckPath is the prefix of the Passkey check operations that use the cookie.
const PasskeyCheckPath = "/site/me/passkeys/check"

// PasskeyLoginPath is the prefix of the login operations that use the cookie.
const PasskeyLoginPath = "/site/auth/passkey"

// PasskeyCeremony wraps the sealed ceremony in its cookie: httpOnly, SameSite=Strict (a
// ceremony begins and finishes in one page), living for ttl and sent only to path.
func (s *Sessions) PasskeyCeremony(path, value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name: PasskeyCeremonyCookie, Value: value, Path: path, MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	}
}
