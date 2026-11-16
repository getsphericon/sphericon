// Package signup is the Sign-up (GLOSSARY, ADR 0030, ADR 0031, ADR 0102), a domain module of
// identity without a table, and every step that makes a User Verified: the Sign-up itself, with
// verification required or off, the verification link, the reset link of an Unverified User, an
// Owner made from a shell (ADR 0042), a provider sign-in that creates or takes over a User, and the
// completion of a pending sign-up. It sits above the users module and the Second factors, neither
// of which owns the Sign-up: each step writes the User's part through the users module's ...In
// functions, which hold the rules of the User's rows, and, on the same transaction, creates the
// User's first Workspace with its Owner Membership, the Send cap it starts with (ADR 0067) and its
// `workspace.create` Audit entry (memberships.CreateWorkspaceIn). A Verified User's later
// Workspaces are internal/usecase/workspaceview's.
//
// The sign-in through a Sign-in provider is here whole (ADR 0111): the provider's answer, the
// Invitation the flow began from, the Sign-up items, the decision, the Second factor and the
// `user.login` entry, so its driver only turns what happened into a cookie or a redirect.
//
// It takes the raw client's transactions: a User and its first Workspace belong to no Workspace
// when the step starts, so there is no *ent.Scoped to take.
package signup

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// Module signs Users up and makes them Verified with their first Workspace.
type Module struct {
	bus       *events.Bus
	users     *users.Module
	consents  *consentrecords.Module
	links     *users.Links
	passwords *credentials.Hasher
	// defaultSendCap gives the Send cap a first Workspace starts with.
	defaultSendCap *suspension.DefaultSendCap
	// tokens revokes the User's Personal tokens and Connections, and factors clears their Second
	// factor and Passkeys, when the first proof of an address drops them (ADR 0170); factors also
	// says whether a User signing in through a provider has a Second factor due (ADR 0020).
	tokens  *apitokens.Module
	factors *secondfactors.Module
	// signUp is what a Sign-up needs besides the modules.
	signUp SignUpConfig
	// providers is what a sign-in through a Sign-in provider needs besides the users module.
	providers SignInConfig
	// now is the Instance clock a pending sign-up is checked against.
	now func() time.Time
	// lookup finds the User that holds an address, whatever its case; ok is false when none. New
	// sets it to the users module's; the race test of two Sign-ups of one new address replaces it
	// (export_test.go) to replay step 3 of SignUp.
	lookup func(ctx context.Context, email string) (u *ent.User, ok bool, err error)
}

// New builds the module: the bus opens its transactions, the users module writes the User's part
// of each step, consents checks the answers to the Sign-up items and makes their Consent records
// (consentrecords.Module.Check), links reads the verification and reset links, passwords hashes
// the password of an Owner made from a shell, defaultSendCap says the Send cap a Workspace starts
// with (suspension.DefaultSendCap.StartingIn, ADR 0067, ADR 0146), tokens and factors drop the
// credentials set up before the first proof of an address (ADR 0170), cfg is what a Sign-up needs
// (ADR 0030, ADR 0031), signIn what a sign-in through a Sign-in provider needs (ADR 0048) and now
// is the Instance clock.
func New(bus *events.Bus, usersModule *users.Module, consents *consentrecords.Module, links *users.Links,
	passwords *credentials.Hasher, defaultSendCap *suspension.DefaultSendCap, tokens *apitokens.Module,
	factors *secondfactors.Module, cfg SignUpConfig, signIn SignInConfig, now func() time.Time,
) *Module {
	m := &Module{
		bus: bus, users: usersModule, consents: consents, links: links, passwords: passwords,
		defaultSendCap: defaultSendCap, tokens: tokens, factors: factors, signUp: cfg, providers: signIn, now: now,
	}
	m.lookup = m.lookupUser
	return m
}
