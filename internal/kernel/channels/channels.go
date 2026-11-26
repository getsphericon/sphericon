// Package channels names the delivery media (GLOSSARY: Channel): the one list a Broadcast, an
// Integration, a Suppression, an outbound message, an Unsubscribe and a Confirmation store and the
// contract's channel enums carry. It is a leaf, so the ent schema types those columns with it.
package channels

// Channel identifies a delivery medium.
//
// It is a closed enum (rule 19 of docs/standards/review-rules.md). A new Channel is added by adding
// its constant, then following the lint errors, which name each switch and map that does not handle
// it (rank below among them), and those of `mise run generate`, where goverter maps a Channel to the
// contract's enums.
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

// rank is every Channel with its place in the order the columns list them.
var rank = map[Channel]int{Email: 0, Telegram: 1, SMS: 2}

// Values is every Channel as a string, for the ent enum columns typed with it.
func (Channel) Values() []string {
	values := make([]string, len(rank))
	for c, i := range rank {
		values[i] = string(c)
	}
	return values
}

// Valid reports whether c is a Channel: one a stored or signed value may name.
func (c Channel) Valid() bool {
	_, ok := rank[c]
	return ok
}
