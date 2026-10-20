package signin

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// google is Google's OpenID Connect sign-in. The id_token carries the profile, so the
// access token is never used.
func google() Entry {
	return Entry{
		Name:        "google",
		DisplayName: "Google",
		Scopes:      []string{"openid", "email", "profile"},
		Endpoints: Endpoints{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Issuer:   "https://accounts.google.com",
			JWKSURL:  "https://www.googleapis.com/oauth2/v3/certs",
		},
		Profile: oidcProfile,
		Trust:   googleTrust,
	}
}

// oidcProfile reads the Profile from the id_token of an OIDC provider, checking its
// signature, issuer, audience, expiry and the flow's nonce.
func oidcProfile(ctx context.Context, x Exchange) (Profile, error) {
	raw, _ := x.Token.Extra("id_token").(string)
	if raw == "" {
		return Profile{}, errors.New("the token answer carries no id_token")
	}
	token, err := x.Verifier.Verify(ctx, raw)
	if err != nil {
		return Profile{}, fmt.Errorf("id_token: %w", err)
	}
	if token.Nonce != x.Nonce {
		return Profile{}, errors.New("id_token: the nonce is not the flow's")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		HostedDomain  string `json:"hd"`
	}
	if err := token.Claims(&claims); err != nil {
		return Profile{}, fmt.Errorf("id_token claims: %w", err)
	}
	return Profile{
		Subject:       token.Subject,
		Name:          strings.TrimSpace(claims.Name),
		Email:         strings.TrimSpace(claims.Email),
		EmailVerified: claims.EmailVerified,
		HostedDomain:  claims.HostedDomain,
	}, nil
}

// googleTrust accepts Google's `email_verified` only where Google is authoritative for the
// address: a Gmail address, or an account of a hosted domain (the `hd` claim). For any
// other address (a Google account created over someone else's mailbox) the claim is weak
// evidence, so the address is refused and the person signs up by email instead. This is
// Google's own guidance for relying on the claim.
func googleTrust(p Profile) Reason {
	if p.Email == "" || !p.EmailVerified {
		return ReasonEmailUnverified
	}
	if p.HostedDomain != "" || isGmail(p.Email) {
		return ""
	}
	return ReasonEmailNotTrusted
}

func isGmail(email string) bool {
	at := strings.LastIndexByte(email, '@')
	return at >= 0 && strings.EqualFold(email[at+1:], "gmail.com")
}
