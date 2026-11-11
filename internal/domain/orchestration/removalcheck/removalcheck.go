// Package removalcheck is the Removal check (GLOSSARY "Removal check", ADR 0049, ADR 0099): before
// a row is removed, or changed in a way that would break what uses it (a Custom field's type), it
// asks every module that may hold the row whether it does, and refuses with a *liveuse.Error naming
// them. A Tag and a Custom field are held through Definition use (a Segment's Condition, the live
// version or the draft of a kept Automation or Rule, the Condition an armed Broadcast copied), an
// Integration and a Sending domain through Live use (the work that will still send through them).
//
// It sits at the top of orchestration because the holders are the Segments below it and the
// Broadcasts, Automations and Rules beside it, which the modules of the removed rows (tags,
// contacts, integrations, sendingdomains) know nothing of. It is the one list of their readers,
// which each owner keeps in a subpackage <module>use (segmentsuse, rulesuse, automationsuse,
// broadcastsuse) that easement opens to this package alone (ADR 0110), so a new holder is added
// here and every removal that checks the row sees it. The use cases that remove a checked row
// (tagdeletion, customfieldretyping, integrationdeletion, sendingdomaindeletion) call it under the
// Workspace lock, which every write that moves a use takes too. Its methods and the readers take
// the scope of that lock, *ent.WorkspaceLocked (ADR 0108), so none of them runs without it.
package removalcheck

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments/segmentsuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations/automationsuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts/broadcastsuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules/rulesuse"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Module is the Removal check. Every call receives the scope of the transaction the removal runs
// in, under the Workspace's lock (ADR 0099).
type Module struct {
	senders *messaging.Resolver
}

// New builds the Removal check over the resolver a send builds its sender with: a Sending domain's
// check reads from it the From an Integration sends with.
func New(senders *messaging.Resolver) *Module { return &Module{senders: senders} }

// Tag checks that nothing holds the Tag with the name, in Definition use: a Segment's Condition
// (archived ones too), a step of a kept Automation's live version or draft, a kept Rule's live
// version or draft (a Condition leaf or a tag Action) and the Condition an armed Broadcast copied
// from its Segment (ADR 0065). Each names the Tag, not its id, so it would silently stop matching
// or applying. It returns nil, or a *liveuse.Error naming them in that order.
func (m *Module) Tag(ctx context.Context, s *ent.WorkspaceLocked, name string) error {
	segs, err := segmentsuse.UsingTag(ctx, s, name)
	if err != nil {
		return err
	}
	autos, err := automationsuse.UsingTag(ctx, s, name)
	if err != nil {
		return err
	}
	kept, err := rulesuse.UsingTag(ctx, s, name)
	if err != nil {
		return err
	}
	armed, err := broadcastsuse.UsingTag(ctx, s, name)
	if err != nil {
		return err
	}
	return refusal(liveuse.SubjectTag, slices.Concat(liveuse.Segments(segs), liveuse.Automations(autos), liveuse.Rules(kept), liveuse.Broadcasts(armed)))
}

// CustomFieldRetype checks that nothing tests the Custom field key with a Condition leaf a field of
// type t does not take, in Definition use: a Segment, a kept Rule and an armed Broadcast, whose
// Conditions would stop compiling (ADR 0006). An Automation holds no Condition, and a leaf every
// type takes (notNull) does not hold the type. It returns nil, or a *liveuse.Error naming them in
// that order.
func (m *Module) CustomFieldRetype(ctx context.Context, s *ent.WorkspaceLocked, key string, t customfield.Type) error {
	segs, err := segmentsuse.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return err
	}
	kept, err := rulesuse.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return err
	}
	armed, err := broadcastsuse.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return err
	}
	return refusal(liveuse.SubjectCustomField, slices.Concat(liveuse.Segments(segs), liveuse.Rules(kept), liveuse.Broadcasts(armed)))
}

// Integration checks that no Live use sends through the Integration with the id (ADR 0099): a
// scheduled or sending Broadcast, the live version of an active Automation or a version an
// unfinished Enrollment stands on, whose send goes through it by the resolution a send takes
// (messaging.Through): the Integration it names or, naming none, its own Channel's default. It
// returns nil, or a *liveuse.Error naming that work, Broadcasts first, then Automations.
func (m *Module) Integration(ctx context.Context, s *ent.WorkspaceLocked, id int64) error {
	sends, err := liveSends(ctx, s)
	if err != nil {
		return err
	}
	through := lo.Filter(sends, func(send liveuse.Send, _ int) bool { return send.Uses(id) })
	return refusal(liveuse.SubjectIntegration, liveuse.SendUsers(through))
}

// SendingDomain checks that no Live use sends email from an address on the domain (ADR 0099). A
// domain is used through the domain of a From address, not an id: the From a send's email goes out
// with, its own or, without one, the From of the Integration it goes through (an Automation step
// carries none), by the resolution a send takes (messaging.Through, messaging.EffectiveFrom). It
// returns nil, or a *liveuse.Error naming that work, Broadcasts first, then Automations.
func (m *Module) SendingDomain(ctx context.Context, s *ent.WorkspaceLocked, domain string) error {
	sends, err := liveSends(ctx, s)
	if err != nil {
		return err
	}
	senders := map[int64]messaging.EmailSender{}
	var onDomain []liveuse.Send
	for _, send := range sends {
		if send.Channel != channels.Email {
			continue
		}
		var sender messaging.EmailSender
		if send.Integration != nil {
			if sender = senders[send.Integration.ID]; sender == nil {
				if sender, err = m.senders.EmailSender(ctx, s.Scope(), send.Integration); err != nil {
					return err
				}
				senders[send.Integration.ID] = sender
			}
		}
		if from, _ := messaging.EffectiveFrom(sender, send.From, ""); messaging.DomainOf(from) == domain {
			onDomain = append(onDomain, send)
		}
	}
	return refusal(liveuse.SubjectSendingDomain, liveuse.SendUsers(onDomain))
}

// liveSends is the Live use that sends (ADR 0099): the Broadcasts' sends, then the Automations'.
func liveSends(ctx context.Context, s *ent.WorkspaceLocked) ([]liveuse.Send, error) {
	sent, err := broadcastsuse.LiveSends(ctx, s)
	if err != nil {
		return nil, err
	}
	stepped, err := automationsuse.LiveSends(ctx, s)
	if err != nil {
		return nil, err
	}
	return slices.Concat(sent, stepped), nil
}

// refusal is the refusal of a removal of the subject the users hold, or nil when none does.
func refusal(what liveuse.Subject, users []liveuse.User) error {
	if len(users) == 0 {
		return nil
	}
	return &liveuse.Error{What: what, Users: users}
}
