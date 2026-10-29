//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// colleaguePassword is the password every Colleague signs up with.
const colleaguePassword = "colleague-password-1"

// Colleague is a person who signed up with an address on the Workspace's Join domain and
// followed the verify link from Mailpit.
type Colleague struct {
	// Email is the address they signed up with.
	Email string
	// Landing is the Workspace the verify link opened.
	Landing siteapi.WorkspaceId
	// site is the SPA's API under the session the verify link started.
	site *siteapi.Client
}

// AddVerifiedJoinDomain is the Owner adding a domain unique to this test in the members
// settings and verifying it: the dev TXT lookup answers the record from the stored token, as
// a published record would. It returns the domain.
func (w *Workspace) AddVerifiedJoinDomain() string {
	w.t.Helper()
	ctx := w.t.Context()
	domain := "join-" + uniq() + ".e2e.test"
	res, err := w.site.SiteJoinDomainsAdd(ctx, &siteapi.SiteAddJoinDomainInput{Domain: domain},
		siteapi.SiteJoinDomainsAddParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	added := ok[siteapi.SiteJoinDomainResource](w.t, "add join domain", res, err)

	verified, err := w.site.SiteJoinDomainsVerify(ctx,
		siteapi.SiteJoinDomainsVerifyParams{WorkspaceId: siteapi.WorkspaceId(w.publicID), ID: added.ID})
	row := ok[siteapi.SiteJoinDomainResource](w.t, "verify join domain", verified, err)
	require.True(w.t, row.Verified, "the domain %s reads as verified", domain)
	return domain
}

// EnableDomainJoin is the Owner switching domain join on in the Domain access section.
func (w *Workspace) EnableDomainJoin() {
	w.t.Helper()
	res, err := w.site.SiteDomainJoinSet(w.t.Context(), &siteapi.SiteSetDomainJoinInput{Enabled: true},
		siteapi.SiteDomainJoinSetParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	got := ok[siteapi.SiteDomainJoinResource](w.t, "switch domain join on", res, err)
	require.True(w.t, got.Enabled)
}

// SignUpColleague signs up a person on the Join domain through the real flow: the
// Sign-up form, the verify mail from Mailpit and the link in it.
func (w *Workspace) SignUpColleague(domain string) *Colleague {
	w.t.Helper()
	ctx := w.t.Context()
	email := "colleague-" + uniq() + "@" + domain
	anonymous, err := siteapi.NewClient(w.env.BaseURL+"/site", cookieSource(""))
	require.NoError(w.t, err)

	signedUp, err := anonymous.SiteAuthSignUp(ctx, &siteapi.SiteSignUpInput{
		Name: "Colleague", Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(colleaguePassword),
	})
	ok[siteapi.SiteAuthSignUpAccepted](w.t, "sign up", signedUp, err)

	mail := w.Inbox.Wait(Match{To: email, Subject: "Verify your Sphericon email"})
	verified, err := anonymous.SiteAuthVerifyEmail(ctx,
		&siteapi.SiteVerifyEmailInput{Token: tokenInMail(w, mail), Password: siteapi.NewOptString(colleaguePassword)},
		siteapi.SiteAuthVerifyEmailParams{})
	landed := ok[siteapi.SiteVerifyEmailResultHeaders](w.t, "follow the verify link", verified, err)

	cookie, err := http.ParseSetCookie(landed.SetCookie)
	require.NoError(w.t, err)
	session, err := siteapi.NewClient(w.env.BaseURL+"/site", cookieSource(cookie.Value))
	require.NoError(w.t, err)
	return &Colleague{Email: email, Landing: landed.Response.LandingWorkspaceId, site: session}
}

// tokenInMail reads the link token out of a verify mail.
func tokenInMail(w *Workspace, m Message) string {
	w.t.Helper()
	_, rest, found := strings.Cut(m.Text, "token=")
	require.True(w.t, found, "the verify mail carries a token link: %q", m.Text)
	token, _, _ := strings.Cut(rest, "\n")
	token, _, _ = strings.Cut(strings.TrimSpace(token), " ")
	unescaped, err := url.QueryUnescape(token)
	require.NoError(w.t, err)
	return unescaped
}

// RequireJoinedByDomain asserts that the Colleague landed in this Workspace and that the
// Owner's members list shows them as a member who joined by domain.
func (w *Workspace) RequireJoinedByDomain(c *Colleague) {
	w.t.Helper()
	ctx := w.t.Context()
	require.Equal(w.t, siteapi.WorkspaceId(w.publicID), c.Landing, "the verify link opens the joined Workspace")

	listed, err := c.site.SiteWorkspacesList(ctx)
	require.NoError(w.t, err)
	ids := make([]string, len(listed))
	for i := range listed {
		ids[i] = string(listed[i].ID)
	}
	require.Contains(w.t, ids, w.publicID, "the Colleague reaches the Workspace")
	require.Len(w.t, ids, 2, "and still has a personal Workspace of their own")

	res, err := w.site.SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	require.NoError(w.t, err)
	members, isList := res.(*siteapi.SiteMembershipsListOKApplicationJSON)
	require.True(w.t, isList, "list members: unexpected response %T", res)
	var joined *siteapi.SiteMembershipResource
	for i := range *members {
		if string((*members)[i].Email) == c.Email {
			joined = &(*members)[i]
		}
	}
	require.NotNil(w.t, joined, "%s is a member", c.Email)
	require.Equal(w.t, siteapi.SiteMembershipRoleMember, joined.Role)
	require.Equal(w.t, siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSourceDomain), joined.Source)
}
