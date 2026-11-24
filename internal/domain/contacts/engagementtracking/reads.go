package engagementtracking

import (
	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// ReadsWhere is a Contact's consent in SQL, as recorded (the Workspace's default aside): it keeps
// the Contacts whose consent on channel reads as state. On email it is the consent of the Contact's
// email address, on sms of its phone (the number or, once erased, its hash), and on telegram of
// its own chats (a Visitor's reaches no Contact, ADR 0171), which read denied when any is denied
// and granted when the Contact has one and every one is granted. A Contact without a Destination
// on the Channel reads as neither. Every form is an EXISTS, never NULL, so a negation of it keeps
// exactly the Contacts it leaves out.
func ReadsWhere(channel channels.Channel, state State) func(*sql.Selector) {
	return func(outer *sql.Selector) {
		outer.Where(readsOf(outer, channel, state))
	}
}

// readsOf is ReadsWhere's predicate over the outer contacts query.
func readsOf(outer *sql.Selector, channel channels.Channel, state State) *sql.Predicate {
	workspace := outer.C(contact.FieldWorkspaceID)
	switch channel {
	case channels.Email:
		return sql.Exists(recordedOf(workspace, outer.C(contact.FieldEmail), channel, state))
	case channels.SMS:
		return sql.Exists(recordedOf(workspace, outer.C(contact.FieldPhone), channel, state))
	case channels.Telegram:
		return chatsRead(outer, state)
	}
	return sql.False()
}

// chatsRead is whether the Contact's own Telegram chats read as state: denied when any chat is
// denied; granted when it has a chat and none lacks a grant (a chat's consent is one state, so
// none is denied then either).
func chatsRead(outer *sql.Selector, state State) *sql.Predicate {
	if state == Denied {
		chats := ownChats(outer)
		chats.Where(sql.Exists(recordedOf(chats.C(binding.FieldWorkspaceID), chats.C(binding.FieldDestination), channels.Telegram, Denied)))
		return sql.Exists(chats)
	}
	ungranted := ownChats(outer)
	ungranted.Where(sql.NotExists(recordedOf(ungranted.C(binding.FieldWorkspaceID), ungranted.C(binding.FieldDestination), channels.Telegram, Granted)))
	return sql.And(sql.Exists(ownChats(outer)), sql.NotExists(ungranted))
}

// ownChats selects the Telegram Bindings of the outer query's Contact.
func ownChats(outer *sql.Selector) *sql.Selector {
	chats := sql.Table(binding.Table).As("tracking_chats")
	return sql.Select(chats.C(binding.FieldID)).From(chats).Where(sql.And(
		sql.ColumnsEQ(chats.C(binding.FieldContactID), outer.C(contact.FieldID)),
		sql.EQ(chats.C(binding.FieldChannel), string(channels.Telegram)),
	))
}

// recordedOf selects the consent rows of state on channel of the Destination in destination (a
// column of the outer query holding its stored form) in the Workspace in workspace. On sms a
// refusal is kept by the number or, once its Contact was erased, by the number's hash (phone.Hash,
// here in SQL, as the eligibility layers match an opt-out), so it holds when the number comes back.
func recordedOf(workspace, destination string, channel channels.Channel, state State) *sql.Selector {
	consents := sql.Table(engagementtrackingconsent.Table)
	sameDestination := sql.ColumnsEQ(consents.C(engagementtrackingconsent.FieldDestination), destination)
	if channel == channels.SMS {
		// raw sql: the destination correlation, matched against the number or its SHA-256 hex.
		sameDestination = sql.P(func(b *sql.Builder) {
			b.WriteString(consents.C(engagementtrackingconsent.FieldDestination)).WriteString(" IN (").
				WriteString(destination).WriteString(", encode(sha256(convert_to(").
				WriteString(destination).WriteString(", 'UTF8')), 'hex'))")
		})
	}
	return sql.Select(consents.C(engagementtrackingconsent.FieldID)).From(consents).Where(sql.And(
		// Every Workspace-owned table names its Workspace in the same column (WorkspaceMixin).
		sql.ColumnsEQ(consents.C(engagementtrackingconsent.FieldWorkspaceID), workspace),
		sameDestination,
		sql.EQ(consents.C(engagementtrackingconsent.FieldChannel), string(channel)),
		sql.EQ(consents.C(engagementtrackingconsent.FieldState), string(state)),
	))
}
