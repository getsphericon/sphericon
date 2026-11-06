package users

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
)

// NewUser is a User a Sign-up creates (internal/usecase/workspacecreation), with the Consent
// records its answers make, written in the transaction that creates the User.
type NewUser struct {
	Name         string
	Email        string
	PasswordHash string
	// SurveyDue marks the User as owing the Sign-up survey.
	SurveyDue bool
	Consents  consentrecords.Consents
}

// BrowserHash is the hash of a Sign-up browser cookie's value, empty for no cookie.
func BrowserHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// SameBrowser reports whether a request's Sign-up browser cookie value is the one whose hash a
// verification token carries. A token minted without a hash (a link asked for from a browser
// that holds no cookie) matches no browser.
func SameBrowser(tokenHash, cookieValue string) bool {
	if tokenHash == "" || cookieValue == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHash), []byte(BrowserHash(cookieValue))) == 1
}
