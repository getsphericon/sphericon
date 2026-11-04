// Package optout is where a person's stop is read and recorded in the Channel itself (ADR 0077):
// the STOP and START keywords every text Channel shares, and the Telegram and SMS opt-outs that
// every way of stopping there (a keyword, the unsubscribe button or link, blocking the bot, a send
// the provider refuses) ends in.
package optout

import "strings"

// stopKeywords are the messages that are a stop by themselves, lower-cased. The list is the core's,
// shared by every Channel where a person writes to us (Telegram and SMS); a Channel adds its own
// command (`/stop` on Telegram) next to it.
var stopKeywords = map[string]bool{
	"stop":        true,
	"stopall":     true,
	"unsubscribe": true,
	"cancel":      true,
	"end":         true,
	"quit":        true,
	"стоп":        true,
	"отписаться":  true,
}

// IsStopKeyword reports whether text is exactly a STOP keyword, ignoring case and the space around
// it. A keyword inside a longer message ("please don't stop") is not one.
func IsStopKeyword(text string) bool {
	return stopKeywords[strings.ToLower(strings.TrimSpace(text))]
}

// startKeywords are the messages that lift a stop by themselves, lower-cased: the person asks to
// receive again (ADR 0077).
var startKeywords = map[string]bool{
	"start":  true,
	"unstop": true,
}

// IsStartKeyword reports whether text is exactly a START keyword, read as IsStopKeyword reads a stop.
func IsStartKeyword(text string) bool {
	return startKeywords[strings.ToLower(strings.TrimSpace(text))]
}
