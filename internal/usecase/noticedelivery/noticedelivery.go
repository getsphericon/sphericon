// Package noticedelivery is the use case of Notice delivery (GLOSSARY, ADR 0105): what sphericon
// tells a person on its own behalf when one of its domain events says something happened that they
// must know of. A User is sent their account's links and notices (a verification link, a password
// reset, a deleted Passkey, …), a Member their Workspace's invitation and Second factor deadline,
// and the Owners of a Workspace what happened to its standing and its configuration (a suspension,
// a lost domain, a disabled Webhook endpoint, …).
//
// The events are published by the modules that make the change, in its transaction, and know
// nothing of who is told or how; Issue is called by the Notices' event consumer and answers, for
// one event, the Notices it calls for, each addressed and worded: the recipients and whatever the
// copy needs are read when the event is handled, and a link that signs in or proves something is
// minted then, bound to the User as they are. The consumer hands each Notice to the System sender
// (internal/jobs), one send job per recipient, so a failed send is retried on its own.
//
// An event of a User's account belongs to no Workspace, and a Workspace's Owners are read across
// the Instance by the Workspace's id, so there is no *ent.Scoped to take: this use case holds the
// raw *ent.Client, one of the closed list of docs/architecture/workspace-scoping.md (Workspace scoping).
package noticedelivery

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
)

// Notice is one message to one person: the address it goes to, its subject and its plain-text
// body, worded in the Instance's locale.
type Notice struct {
	To      string
	Subject string
	Text    string
}

// Config is what the copy of the Notices needs from the Instance's configuration.
type Config struct {
	// AppURL is the address of the app, which every link of a Notice opens.
	AppURL string
	// Support is where an Owner appeals a suspension or a warning, "" for none.
	Support string
	// TrackingTarget is the CNAME target a Tracking host points at, which the Tracking host
	// notices name.
	TrackingTarget string
}

// UseCase issues the Notices of the domain events.
type UseCase struct {
	client    *ent.Client
	links     *users.Links
	factors   *secondfactors.Module
	providers *signinprovider.Providers
	cfg       Config
	now       func() time.Time
}

// New builds Notice delivery over the raw client, the links of the account mails, the Second
// factors (whose deadlines the Two-factor notices name), the Sign-in providers (which name a linked
// one) and the Instance's configuration; now is the Instance clock.
func New(client *ent.Client, links *users.Links, factors *secondfactors.Module, providers *signinprovider.Providers, cfg Config, now func() time.Time) *UseCase {
	return &UseCase{client: client, links: links, factors: factors, providers: providers, cfg: cfg, now: now}
}

// Issue answers the Notices ev calls for (the Notices' consumer wants only an event that calls for
// some), none when it calls for none (a request that is not
// delivered, a User gone since, a suspension lifted since). A suspension's notice is recorded on
// the Workspace (workspaces.MarkAbuseFreezeNotified) when it is issued. An error leaves the event
// to be handled again: Issue reads and mints nothing it cannot read and mint again.
func (u *UseCase) Issue(ctx context.Context, ev events.DomainEvent) ([]Notice, error) {
	switch e := ev.(type) {
	case *events.LinkRequested:
		return u.link(ctx, e)
	case *events.AddressClaimed:
		return u.addressClaimed(e), nil
	case *events.UserSignedUp:
		return []Notice{welcome(e)}, nil
	case *events.IdentityLinked:
		return u.identityLinked(ctx, e)
	case *events.UserDeleted:
		return []Notice{userDeleted(e)}, nil
	case *events.PasskeyChange:
		return passkeyChanged(e)
	case *events.InvitationCreated:
		return u.invitation(ctx, e)
	case *events.SecondFactorRequired:
		return u.secondFactorRequired(ctx, e)
	case *events.SecondFactorReminderDue:
		return u.secondFactorReminder(ctx, e)
	case *events.WorkspaceSuspended:
		return u.suspended(ctx, e)
	default:
		return u.ownerNotice(ctx, ev)
	}
}

// accountFooter ends every account mail with a link.
func accountFooter() string { return i18n.T(i18n.EmailAuthFooter, nil) }

// appLink is the app's page path, with query q when it is not empty.
func (u *UseCase) appLink(path string, q url.Values) string {
	link := strings.TrimRight(u.cfg.AppURL, "/") + path
	if len(q) > 0 {
		link += "?" + q.Encode()
	}
	return link
}

// linkCopy is the copy of an account mail that carries a one-time link: its subject, the SPA page
// the link opens (which posts the token back) and the line above the link.
type linkCopy struct {
	subject, intro i18n.ID
	path           string
}

// linkCopies is the copy of every link request.
var linkCopies = map[string]linkCopy{
	events.NameUserPasswordResetRequested: {subject: i18n.EmailPasswordResetSubject, path: "/reset-password", intro: i18n.EmailPasswordResetIntro},
	events.NameUserVerificationRequested:  {subject: i18n.EmailEmailVerifySubject, path: "/verify-email", intro: i18n.EmailEmailVerifyIntro},
	events.NameUserEmailChangeRequested:   {subject: i18n.EmailEmailChangeSubject, path: "/confirm-email-change", intro: i18n.EmailEmailChangeIntro},
	events.NameUserDeletionRequested:      {subject: i18n.EmailUserDeletionSubject, path: "/delete-account", intro: i18n.EmailUserDeletionIntro},
}

// link is the mail of a link request that is delivered: the link is minted now, bound to the User
// as they are (a password changed since the request ends it).
func (u *UseCase) link(ctx context.Context, e *events.LinkRequested) ([]Notice, error) {
	c, ok := linkCopies[e.Action]
	if !ok {
		return nil, fmt.Errorf("notice: unknown link request %q", e.Action)
	}
	if !e.Deliver || e.UserID == 0 {
		return nil, nil
	}
	usr, found, err := u.user(ctx, e.UserID)
	if err != nil || !found {
		return nil, err
	}
	token, err := u.mint(usr, e)
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf("%s\n\n%s\n\n%s\n", i18n.T(c.intro, nil), u.appLink(c.path, url.Values{"token": {token}}), accountFooter())
	return []Notice{{To: e.Email, Subject: i18n.T(c.subject, nil), Text: body}}, nil
}

// mint is the token of the link e asks for, for usr.
func (u *UseCase) mint(usr *ent.User, e *events.LinkRequested) (string, error) {
	switch e.Action {
	case events.NameUserPasswordResetRequested:
		return u.links.PasswordResetLink(usr)
	case events.NameUserVerificationRequested:
		return u.links.VerificationLink(usr, e.BrowserHash)
	case events.NameUserEmailChangeRequested:
		return u.links.EmailChangeLink(usr, e.Email)
	default:
		return u.links.DeletionLink(usr)
	}
}

// user is the User id, found false when it is gone.
func (u *UseCase) user(ctx context.Context, id int64) (*ent.User, bool, error) {
	usr, err := u.client.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("notice: load user %d: %w", id, err)
	}
	return usr, true, nil
}

// addressClaimed is the notice a Sign-up or an email change to a Taken address sends its owner. It
// carries no token, so it can never be a way in.
func (u *UseCase) addressClaimed(e *events.AddressClaimed) []Notice {
	if !e.Deliver {
		return nil
	}
	body := fmt.Sprintf("%s\n\n%s %s\n%s %s\n\n%s\n",
		i18n.T(i18n.EmailAddressTakenIntro, nil),
		i18n.T(i18n.EmailAddressTakenSignIn, nil), u.appLink("/login", nil),
		i18n.T(i18n.EmailAddressTakenReset, nil), u.appLink("/forgot-password", nil),
		accountFooter())
	return []Notice{{To: e.Email, Subject: i18n.T(i18n.EmailAddressTakenSubject, nil), Text: body}}
}

// welcome greets a User whose Sign-up completed, to an address that is now proven.
func welcome(e *events.UserSignedUp) Notice {
	greeting := e.Name
	if greeting == "" {
		greeting = i18n.T(i18n.EmailWelcomeGreetingFallback, nil)
	}
	return Notice{
		To:      e.Email,
		Subject: i18n.T(i18n.EmailWelcomeSubject, nil),
		Text:    i18n.T(i18n.EmailWelcomeBody, map[string]any{"Greeting": greeting}),
	}
}

// identityLinked tells the owner of the account that a Sign-in provider was linked silently, with
// the link that undoes it.
func (u *UseCase) identityLinked(ctx context.Context, e *events.IdentityLinked) ([]Notice, error) {
	usr, found, err := u.user(ctx, e.UserID)
	if err != nil || !found {
		return nil, err
	}
	token, err := u.links.NotMeLink(usr, e.IdentityID)
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n",
		i18n.T(i18n.EmailIdentityLinkedIntro, map[string]any{"Provider": u.providerName(e.Provider)}),
		i18n.T(i18n.EmailIdentityLinkedNotMe, nil), u.appLink("/signin-not-me", url.Values{"token": {token}}),
		accountFooter())
	return []Notice{{To: e.Email, Subject: i18n.T(i18n.EmailIdentityLinkedSubject, nil), Text: body}}, nil
}

// providerName is what the Sign-in provider name is called to a person.
func (u *UseCase) providerName(name string) string {
	if u.providers == nil {
		return name
	}
	return u.providers.DisplayName(name)
}

// userDeleted is the one mail a User deletion sends the address: the owner of the mailbox learns
// of a deletion they did not make. No token and no link.
func userDeleted(e *events.UserDeleted) Notice {
	body := fmt.Sprintf("%s\n\n%s\n", i18n.T(i18n.EmailUserDeletedIntro, nil), i18n.T(i18n.EmailUserDeletedNotMe, nil))
	return Notice{To: e.Email, Subject: i18n.T(i18n.EmailUserDeletedSubject, nil), Text: body}
}

// passkeyCopy is the subject and intro of a Passkey notice by the change it reports.
var passkeyCopy = map[string][2]i18n.ID{
	events.NameUserPasskeyAdded:   {i18n.EmailPasskeyAddedSubject, i18n.EmailPasskeyAddedIntro},
	events.NameUserPasskeyRemoved: {i18n.EmailPasskeyRemovedSubject, i18n.EmailPasskeyRemovedIntro},
}

// passkeyChanged tells the owner of the account that a Passkey was added or removed. No token and
// no link.
func passkeyChanged(e *events.PasskeyChange) ([]Notice, error) {
	c, ok := passkeyCopy[e.Action]
	if !ok {
		return nil, fmt.Errorf("notice: unknown passkey change %q", e.Action)
	}
	body := fmt.Sprintf("%s\n\n%s\n", i18n.T(c[1], map[string]any{"Name": e.Name}), i18n.T(i18n.EmailPasskeyChangedNotMe, nil))
	return []Notice{{To: e.Email, Subject: i18n.T(c[0], nil), Text: body}}, nil
}

// invitation is the mail of an Invitation, with its one-time link. The Workspace and the inviter
// are named as they are now; an inviter gone since is "Someone".
func (u *UseCase) invitation(ctx context.Context, e *events.InvitationCreated) ([]Notice, error) {
	ws, err := u.client.Workspace.Get(ctx, e.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("notice: load workspace %d: %w", e.WorkspaceID, err)
	}
	who := "Someone"
	if e.InvitedBy != 0 {
		inviter, found, err := u.user(ctx, e.InvitedBy)
		if err != nil {
			return nil, err
		}
		if found && inviter.Name != "" {
			who = inviter.Name
		}
	}
	body := fmt.Sprintf(
		"%s invited you to join the \"%s\" workspace on sphericon.\n\nFollow this link to accept:\n\n%s\n\nIf you weren't expecting this, you can ignore this email.\n",
		who, ws.Name, u.appLink("/invitations/"+e.Token, nil),
	)
	return []Notice{{To: e.Email, Subject: fmt.Sprintf("You're invited to %s on sphericon", ws.Name), Text: body}}, nil
}

// secondFactorSetupPath is the SPA page where a User enrolls a Second factor.
const secondFactorSetupPath = "/account/security"

// secondFactorRequired mails every Member of the Workspace who has no Second factor their deadline.
// The Memberships are read now, so a requirement switched off again in between mails no one.
func (u *UseCase) secondFactorRequired(ctx context.Context, e *events.SecondFactorRequired) ([]Notice, error) {
	members, err := u.client.Scoped(e.WorkspaceID).Membership().Query().WithUser().WithWorkspace().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("notice: load the members of workspace %d: %w", e.WorkspaceID, err)
	}
	deadlines, err := u.factors.Deadlines(ctx, members)
	if err != nil {
		return nil, fmt.Errorf("notice: read the deadlines of workspace %d: %w", e.WorkspaceID, err)
	}
	var notices []Notice
	for _, m := range members {
		if deadline, ok := deadlines[m.ID]; ok {
			notices = append(notices, u.secondFactorMail(i18n.EmailSecondFactorRequiredSubject, i18n.EmailSecondFactorRequiredBody, m, deadline))
		}
	}
	return notices, nil
}

// secondFactorReminder reminds the Member of the claimed reminder that their grace ends at its
// deadline, unless they have a Second factor by now or are no longer a Member.
func (u *UseCase) secondFactorReminder(ctx context.Context, e *events.SecondFactorReminderDue) ([]Notice, error) {
	m, err := u.client.Scoped(e.WorkspaceID).Membership().Query().
		Where(membership.ID(e.MembershipID)).WithUser().WithWorkspace().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notice: load membership %d: %w", e.MembershipID, err)
	}
	active, err := u.factors.Active(ctx, m.UserID)
	if err != nil {
		return nil, fmt.Errorf("notice: read the second factor of user %d: %w", m.UserID, err)
	}
	if active {
		return nil, nil
	}
	return []Notice{u.secondFactorMail(i18n.EmailSecondFactorReminderSubject, i18n.EmailSecondFactorReminderBody, m, e.Deadline)}, nil
}

// secondFactorMail is a Second factor mail to the Membership's User. The deadline is shown in the
// User time zone (ADR 0068); the locale is the Instance's.
func (u *UseCase) secondFactorMail(subject, body i18n.ID, m *ent.Membership, deadline time.Time) Notice {
	usr := m.Edges.User
	data := map[string]any{
		"Workspace": m.Edges.Workspace.Name,
		"Deadline":  mailTime(deadline, usr.Timezone),
		"Link":      u.appLink(secondFactorSetupPath, nil),
	}
	return Notice{To: usr.Email, Subject: i18n.T(subject, data), Text: i18n.T(body, data)}
}

// mailTime is t as a User's mail prints it: the wall clock in the User time zone (ADR 0068) and the
// zone's name, which, unlike an abbreviation, every zone has. A User with none, or with a name the
// zone database no longer knows, reads UTC.
func mailTime(t time.Time, zone *string) string {
	loc, err := reportday.Location(zone)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04") + " " + loc.String()
}

// suspended tells the Owners of a suspended Workspace that its sending is frozen, why and who set
// it (ADR 0007), through the System sender because the Workspace cannot send while suspended (ADR
// 0015), and records on the Workspace that they were told. A suspension lifted since tells no one.
func (u *UseCase) suspended(ctx context.Context, e *events.WorkspaceSuspended) ([]Notice, error) {
	ws, err := u.client.Workspace.Get(ctx, e.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("notice: load workspace %d: %w", e.WorkspaceID, err)
	}
	if !suspension.Is(ws) {
		return nil, nil
	}
	reason := ""
	if ws.SuspensionReason != nil {
		reason = *ws.SuspensionReason
	}
	// The owner never sees a Staff member's id: the CLI and every Staff member are "Sphericon staff".
	actor := i18n.EmailWorkspaceSuspendedActorStaff
	if ws.SuspendedByKind != nil && *ws.SuspendedByKind == workspace.SuspendedByKindSystem {
		actor = i18n.EmailWorkspaceSuspendedActorSystem
	}
	data := map[string]any{"Workspace": ws.Name, "Reason": reason, "Actor": i18n.T(actor, nil), "Support": u.cfg.Support}
	notices, err := u.toOwners(ctx, ws.ID, i18n.EmailWorkspaceSuspendedSubject, i18n.EmailWorkspaceSuspendedBody, data)
	if err != nil {
		return nil, err
	}
	if err := workspaces.MarkAbuseFreezeNotified(ctx, u.client, ws.ID, u.now()); err != nil {
		return nil, fmt.Errorf("notice: record the suspension notice of workspace %d: %w", ws.ID, err)
	}
	return notices, nil
}

// disabledReasons words why the delivery disabled a Webhook endpoint. An Owner disabling one is
// told nothing.
var disabledReasons = map[string]i18n.ID{
	string(webhookendpoint.DisabledReasonGone):              i18n.EmailWebhookDisabledReasonGone,
	string(webhookendpoint.DisabledReasonPersistentFailure): i18n.EmailWebhookDisabledReasonPersistentFailure,
}

// ownerNotice is the notice to a Workspace's Owners of a change of its configuration.
func (u *UseCase) ownerNotice(ctx context.Context, ev events.DomainEvent) ([]Notice, error) {
	switch e := ev.(type) {
	case *events.AbuseWarned:
		ws, err := u.client.Workspace.Get(ctx, e.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("notice: load workspace %d: %w", e.WorkspaceID, err)
		}
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailAbuseWarningSubject, i18n.EmailAbuseWarningBody, map[string]any{
			"Workspace": ws.Name, "Domain": e.Domain, "Metric": e.Metric, "Rate": e.Rate,
			"Count": e.Count, "Total": e.Total, "Limit": e.Limit, "Support": u.cfg.Support,
		})
	case *events.JoinDomainLost:
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailJoinDomainUnverifiedSubject, i18n.EmailJoinDomainUnverifiedBody,
			map[string]any{"Domain": e.Domain, "Record": e.Record})
	case *events.SendingDomainVerificationLost:
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailSendingDomainUnverifiedSubject, i18n.EmailSendingDomainUnverifiedBody,
			map[string]any{"Domain": e.Domain, "Record": e.Record})
	case *events.TrackingDomainChanged:
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailTrackingDomainRegressedSubject, i18n.EmailTrackingDomainRegressedBody,
			map[string]any{"Host": e.Host, "From": e.From, "To": e.To, "Target": u.cfg.TrackingTarget})
	case *events.TrackingDomainReplaced:
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailTrackingDomainReplacedSubject, i18n.EmailTrackingDomainReplacedBody,
			map[string]any{"Host": e.Host, "Old": e.Old, "Target": u.cfg.TrackingTarget})
	case *events.TrackingDomainRemoved:
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailTrackingDomainRetiredGoneSubject, i18n.EmailTrackingDomainRetiredGoneBody,
			map[string]any{"Host": e.Host, "Target": u.cfg.TrackingTarget})
	case *events.WebhookEndpointDisabled:
		ws, err := u.client.Workspace.Get(ctx, e.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("notice: load workspace %d: %w", e.WorkspaceID, err)
		}
		data := map[string]any{
			"Workspace": ws.Name,
			"URL":       e.Origin,
			"Failures":  webhookendpoints.DisableFailures,
			"Window":    webhookendpoints.DisableWindow.String(),
		}
		data["Reason"] = i18n.T(disabledReasons[e.Reason], data)
		return u.toOwners(ctx, e.WorkspaceID, i18n.EmailWebhookDisabledSubject, i18n.EmailWebhookDisabledBody, data)
	}
	return nil, fmt.Errorf("notice: no notice for the event %q", ev.EventName())
}

// toOwners is the notice, rendered from data, to every Owner of the Workspace with an address.
func (u *UseCase) toOwners(ctx context.Context, workspaceID int64, subject, body i18n.ID, data map[string]any) ([]Notice, error) {
	owners, err := memberships.Owners(ctx, u.client.Scoped(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("notice: load the owners of workspace %d: %w", workspaceID, err)
	}
	var notices []Notice
	for _, m := range owners {
		if usr := m.Edges.User; usr != nil && usr.Email != "" {
			notices = append(notices, Notice{To: usr.Email, Subject: i18n.T(subject, data), Text: i18n.T(body, data)})
		}
	}
	return notices, nil
}
