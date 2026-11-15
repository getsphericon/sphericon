// Package signup is the use case of the Sign-up (GLOSSARY, ADR 0030, ADR 0031, ADR 0102) and of
// every step that makes a User Verified: the Sign-up itself, with verification required or off,
// the verification link, the reset link of an Unverified User, an Owner made from a shell (ADR
// 0042), a provider sign-in that creates or takes over a User, and the completion of a pending
// sign-up. Each step writes the User's part through the users module's ...In functions and, on
// the same transaction, creates the User's first Workspace with its Owner Membership, the Send cap
// it starts with (ADR 0067) and its `workspace.create` Audit entry (memberships.CreateWorkspaceIn).
// A Verified User's later Workspaces are internal/usecase/workspaceview's.
//
// The sign-in through a Sign-in provider is here whole (ADR 0111): the provider's answer, the
// Invitation the flow began from, the Sign-up items, the decision, the Second factor and the
// `user.login` entry, so its driver only turns what happened into a cookie or a redirect.
//
// It takes the raw client's transactions: a User and its first Workspace belong to no Workspace
// when the step starts, so there is no *ent.Scoped to take.
package signup

import (
	"time"

	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// UseCase signs Users up and makes them Verified with their first Workspace.
type UseCase struct {
	bus       *events.Bus
	users     *users.Module
	links     *users.Links
	passwords *credentials.Hasher
	// defaultSendCap gives the Send cap a first Workspace starts with.
	defaultSendCap *suspension.DefaultSendCap
	// signUp is what a Sign-up needs besides the users module.
	signUp SignUpConfig
	// providers is what a sign-in through a Sign-in provider needs besides the users module.
	providers SignInConfig
	// now is the Instance clock a pending sign-up is checked against.
	now func() time.Time
}

// New builds the use case: the bus opens its transactions, the users module writes the User's
// part of each step, links reads the verification and reset links, passwords hashes the
// password of an Owner made from a shell, defaultSendCap says the Send cap a Workspace starts with
// (suspension.DefaultSendCap.StartingIn, ADR 0067, ADR 0146), cfg is what a Sign-up needs (ADR 0030, ADR 0031), signIn what a sign-in through a
// Sign-in provider needs (ADR 0048) and now is the Instance clock.
func New(bus *events.Bus, usersModule *users.Module, links *users.Links, passwords *credentials.Hasher,
	defaultSendCap *suspension.DefaultSendCap, cfg SignUpConfig, signIn SignInConfig, now func() time.Time,
) *UseCase {
	u := &UseCase{
		bus: bus, users: usersModule, links: links, passwords: passwords, defaultSendCap: defaultSendCap, providers: signIn, now: now,
	}
	if cfg.Lookup == nil {
		cfg.Lookup = u.lookupUser
	}
	u.signUp = cfg
	return u
}
