package bad

import (
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/medium"
)

func withDefault(c channels.Channel) string {
	switch c { // want `rule 19: the switch on channels.Channel has a default clause`
	case channels.Email:
		return "mail"
	default:
		return "other"
	}
}

func missing(c channels.Channel) string {
	switch c { // want `rule 19: the switch on channels.Channel does not name channels.SMS, channels.Telegram`
	case channels.Email:
		return "mail"
	}
	return ""
}

func derived(m medium.Medium) string {
	switch m { // want `rule 19: the switch on channels.Channel has a default clause`
	case medium.Email:
		return "mail"
	default:
		return "other"
	}
}

func deeper(m medium.Deeper) string {
	switch m { // want `rule 19: the switch on channels.Channel does not name channels.Telegram`
	case medium.Deeper(medium.Email), medium.Deeper(medium.SMS):
		return "mail"
	}
	return ""
}

func alias(m medium.Alias) string {
	switch m { // want `rule 19: the switch on channels.Channel does not name channels.Email`
	case channels.SMS, channels.Telegram:
		return "text"
	}
	return ""
}

var partial = map[channels.Channel]string{ // want `rule 19: the map keyed by channels.Channel does not list channels.SMS`
	channels.Email:    "mail",
	channels.Telegram: "bot",
}

type table map[medium.Medium]int

var derivedTable = table{medium.SMS: 1} // want `rule 19: the map keyed by channels.Channel does not list channels.Email, channels.Telegram`
