// Package setupchecklist reads the Workspace's Setup checklist (GLOSSARY "Setup checklist"):
// four steps, each read off rows the Workspace already has, never ticked by hand. It is a
// point-in-time read model with no state of its own; no step blocks another.
package setupchecklist

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
)

// Status is where one step stands. Only the Sending domain step is ever Pending.
type Status string

const (
	Todo    Status = "todo"
	Pending Status = "pending"
	Done    Status = "done"
)

// Steps are the checklist's four steps, in the order it shows them.
type Steps struct {
	Integration, SendingDomain, Contact, SentEmail Status
}

// Checklist is the read model for one Workspace.
type Checklist struct {
	Steps Steps
	// Dismissed reports that an owner or admin dismissed the checklist for the Workspace.
	Dismissed bool
}

// Complete reports whether every step is done.
func (c Checklist) Complete() bool {
	s := c.Steps
	return s.Integration == Done && s.SendingDomain == Done && s.Contact == Done && s.SentEmail == Done
}

// Read computes the checklist of the Workspace behind s.
func Read(ctx context.Context, s *ent.Scoped) (Checklist, error) {
	var c Checklist
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Checklist{}, err
	}
	c.Dismissed = ws.SetupChecklistDismissedAt != nil

	if c.Steps.Integration, err = exists(s.Integration().Query().Exist(ctx)); err != nil {
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
	verified, err := s.SendingDomain().Query().Where(sendingdomain.Verified(true)).Exist(ctx)
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
