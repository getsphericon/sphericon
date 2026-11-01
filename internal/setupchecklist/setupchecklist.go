// Package setupchecklist reads the Workspace's Setup checklist (GLOSSARY "Setup checklist"):
// four steps, each read off rows the Workspace already has, never ticked by hand; no step blocks
// another. The steps are read now, every time. Its one state is the moment the checklist was first
// found complete: from then on it is done for good, so a step that later regresses does not bring
// the onboarding back (the overview lists that as an item that needs attention).
package setupchecklist

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/dkimstate"
)

// Status is where one step stands. Only the Sending domain step is ever Pending.
type Status string

const (
	Todo    Status = "todo"
	Pending Status = "pending"
	Done    Status = "done"
)

// Steps are the checklist's steps, in the order it shows them.
type Steps struct {
	Integration, SendingDomain, Contact, SentEmail Status
}

// Checklist is the read model for one Workspace.
type Checklist struct {
	Steps Steps
	// CompletedAt is when every step was first found done; nil until then.
	CompletedAt *time.Time
}

// Complete reports whether the checklist was ever found complete.
func (c Checklist) Complete() bool { return c.CompletedAt != nil }

// allDone reports whether every step is done now.
func (s Steps) allDone() bool {
	return s.Integration == Done && s.SendingDomain == Done && s.Contact == Done && s.SentEmail == Done
}

// Recorder stamps the moment a Workspace's checklist is first found complete, once, and returns the
// moment the Workspace holds (internal/accounts).
type Recorder interface {
	MarkSetupComplete(ctx context.Context, s *ent.Scoped, now time.Time) (time.Time, error)
}

// Module reads checklists. Its clock is time.Now.
type Module struct {
	now      func() time.Time
	recorder Recorder
}

func New(recorder Recorder) *Module { return &Module{now: time.Now, recorder: recorder} }

// WithClock returns the module reading now from the given clock.
func (m *Module) WithClock(now func() time.Time) *Module {
	c := *m
	c.now = now
	return &c
}

// Read computes the checklist of the Workspace behind s. The read that first finds every step
// done stamps the Workspace, so the checklist stays complete after.
func (m *Module) Read(ctx context.Context, s *ent.Scoped) (Checklist, error) {
	c, err := steps(ctx, s)
	if err != nil {
		return Checklist{}, err
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Checklist{}, err
	}
	c.CompletedAt = ws.SetupCompletedAt
	if c.CompletedAt != nil || !c.Steps.allDone() {
		return c, nil
	}
	at, err := m.recorder.MarkSetupComplete(ctx, s, m.now())
	if err != nil {
		return Checklist{}, err
	}
	c.CompletedAt = &at
	return c, nil
}

// steps reads where each step stands now.
func steps(ctx context.Context, s *ent.Scoped) (Checklist, error) {
	var c Checklist
	var err error

	// A Test inbox delivers nothing outside, so only a real provider completes the step. Archived
	// Integrations are out of the default query.
	provider := s.Integration().Query().Where(integration.ProviderNEQ(integration.ProviderInbox))
	if c.Steps.Integration, err = exists(provider.Exist(ctx)); err != nil {
		return Checklist{}, err
	}
	if c.Steps.SendingDomain, err = sendingDomain(ctx, s); err != nil {
		return Checklist{}, err
	}
	if c.Steps.Contact, err = exists(s.Contact().Query().Exist(ctx)); err != nil {
		return Checklist{}, err
	}
	// A Test send writes no Outbound message, so only a real send of any surface counts.
	sent := s.OutboundMessage().Query().Where(outboundmessage.StatusEQ(outboundmessage.StatusSent))
	if c.Steps.SentEmail, err = exists(sent.Exist(ctx)); err != nil {
		return Checklist{}, err
	}
	return c, nil
}

func exists(found bool, err error) (Status, error) {
	if err != nil {
		return "", err
	}
	if found {
		return Done, nil
	}
	return Todo, nil
}

func sendingDomain(ctx context.Context, s *ent.Scoped) (Status, error) {
	verified, err := s.SendingDomain().Query().Where(dkimstate.Verified()).Exist(ctx)
	if err != nil {
		return "", err
	}
	if verified {
		return Done, nil
	}
	waiting, err := s.SendingDomain().Query().Exist(ctx)
	if err != nil {
		return "", err
	}
	if waiting {
		return Pending, nil
	}
	return Todo, nil
}
