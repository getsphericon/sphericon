package events

// keptOnErasure is an optional capability of a Projected event whose Event row a Contact's Erasure
// keeps as an anonymous row (ADR 0021), without the Contact, its destination and its personal
// properties: email's and the other Channels' delivery facts, which the deliverability and
// engagement figures count. Every other Event is deleted: a customer's own (tracked, not ours to
// keep), marketing.confirmed and sms.confirmed (consent proof for a person who is gone),
// telegram.received and sms.received (the person's own words) and the Contact's own changes. The
// telegram.* System events are kept like email's, bound and unbound included: they say a Contact's
// chat came and went, and with the Contact and the Destination dropped they are counts; the sms.*
// ones are kept like their email twins, and contact.consent_superseded like an unsubscribe.
type keptOnErasure interface {
	keptOnErasure()
}

func (*ContactCreated) keptOnErasure()       {}
func (*EmailEngagement) keptOnErasure()      {}
func (*EmailDelivery) keptOnErasure()        {}
func (*BindingChange) keptOnErasure()        {}
func (*TelegramSent) keptOnErasure()         {}
func (*TelegramClicked) keptOnErasure()      {}
func (*TelegramUnsubscribed) keptOnErasure() {}
func (*SmsSent) keptOnErasure()              {}
func (*SmsDelivery) keptOnErasure()          {}
func (*SmsClicked) keptOnErasure()           {}
func (*SmsUnsubscribed) keptOnErasure()      {}
func (*ConsentSuperseded) keptOnErasure()    {}

// keptOnErasureActions are the actions whose Events a Contact's Erasure keeps (keptOnErasure).
var keptOnErasureActions = func() map[string]bool {
	kept := map[string]bool{}
	for name, newEvent := range registry {
		_, ok := newEvent().(keptOnErasure)
		kept[name] = ok
	}
	return kept
}()

// KeptOnErasure reports whether a Contact's Erasure keeps an Event of action as an anonymous row
// (keptOnErasure) rather than deleting it; a customer's own action is never kept.
func KeptOnErasure(action string) bool { return keptOnErasureActions[action] }
