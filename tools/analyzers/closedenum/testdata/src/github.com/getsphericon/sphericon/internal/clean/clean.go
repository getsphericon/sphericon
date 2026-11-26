package clean

import (
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/medium"
)

func complete(c channels.Channel) string {
	switch c {
	case channels.Email:
		return "mail"
	case channels.SMS, channels.Telegram:
		return "text"
	}
	return ""
}

func derived(m medium.Medium) string {
	switch m {
	case medium.Email, medium.SMS, medium.Telegram:
		return "any"
	}
	return ""
}

// notAnEnum switches on a string, whatever its values.
func notAnEnum(s string) string {
	switch s {
	case "email":
		return "mail"
	default:
		return ""
	}
}

var full = map[channels.Channel]string{channels.Email: "mail", channels.SMS: "text", channels.Telegram: "bot"}

// empty is a map to fill, not a table.
var empty = map[channels.Channel]int{}

var byName = map[string]channels.Channel{"mail": channels.Email}
