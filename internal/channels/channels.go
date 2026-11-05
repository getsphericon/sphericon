// Package channels names the delivery media (GLOSSARY: Channel): the one list a Broadcast, an
// Integration, a Suppression, an outbound message, an Unsubscribe and a Confirmation store and the
// contract's channel enums carry. It is a leaf, so the ent schema types those columns with it.
package channels

// Channel identifies a delivery medium.
type Channel string

const (
	// Email reaches a person at their email address.
	Email Channel = "email"
	// SMS reaches a person at their phone number through a Workspace's own SMS provider account
	// (Twilio).
	SMS Channel = "sms"
	// Telegram reaches a person through a Workspace's own Telegram bot.
	Telegram Channel = "telegram"
)

// all is every Channel, in the order the columns list them.
var all = []Channel{Email, Telegram, SMS}

// Values is every Channel as a string, for the ent enum columns typed with it.
func (Channel) Values() []string {
	values := make([]string, len(all))
	for i, c := range all {
		values[i] = string(c)
	}
	return values
}
