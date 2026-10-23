package signin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// githubAPI is where GitHub's account and emails are read.
const githubAPI = "https://api.github.com"

// github is GitHub's sign-in: plain OAuth2, so there is no id_token and the access token is
// used to read the account (its numeric id is the subject) and its emails.
func github() Entry {
	return Entry{
		Name:        "github",
		DisplayName: "GitHub",
		Scopes:      []string{"read:user", "user:email"},
		Endpoints: Endpoints{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
			APIURL:   githubAPI,
		},
		Profile: githubProfile,
		Trust:   githubTrust,
	}
}

// githubProfile reads the account and its primary email. The email is the address GitHub
// marks primary, and it counts as proven only when GitHub also marks it verified: the
// account's public profile email proves nothing.
func githubProfile(ctx context.Context, x Exchange) (Profile, error) {
	var account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := githubGet(ctx, x, "/user", &account); err != nil {
		return Profile{}, err
	}
	if account.ID == 0 {
		return Profile{}, errors.New("github account: no id")
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := githubGet(ctx, x, "/user/emails", &emails); err != nil {
		return Profile{}, err
	}
	p := Profile{Subject: strconv.FormatInt(account.ID, 10), Name: strings.TrimSpace(account.Name)}
	if p.Name == "" {
		p.Name = account.Login
	}
	for _, e := range emails {
		if e.Primary {
			p.Email, p.EmailVerified = strings.TrimSpace(e.Email), e.Verified
			break
		}
	}
	return p, nil
}

// githubGet reads one JSON document of GitHub's API with the flow's bearer-authenticated client.
func githubGet(ctx context.Context, x Exchange, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, x.APIURL+path, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-Github-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "sphericon")
	resp, err := x.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("github %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into); err != nil {
		return fmt.Errorf("github %s: %w", path, err)
	}
	return nil
}

// githubTrust accepts the primary email when GitHub verified it, whatever its domain: GitHub
// verifies every address it lists by mail, so it is authoritative for them all.
func githubTrust(p Profile) Reason {
	if p.Email == "" || !p.EmailVerified {
		return ReasonEmailUnverified
	}
	return ""
}
