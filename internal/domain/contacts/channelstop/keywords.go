// Package channelstop is where a Channel stop (GLOSSARY), a person's stop made in the Channel
// itself, is read and recorded (ADR 0077):
// the STOP and START keywords every text Channel shares, and the Telegram and SMS opt-outs that
// every way of stopping there (a keyword, the unsubscribe button or link, blocking the bot, a send
// the provider refuses, one recorded by hand) ends in. It records each through the owners of the
// rows: the Unsubscribe through unsubscribes, and the SMS Confirmation a stop drops through
// confirmations.
package channelstop

import (
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Module records the Channel stops of a Workspace, and reacts to the stops and returns observed
// in a Channel (Subscribers). It publishes in the transaction of the scope it is given, through
// the bus.
type Module struct {
	bus           *events.Bus
	confirmations *confirmations.Module
	unsubs        *unsubscribes.Module
	now           func() time.Time
}

// New builds the Channel stops over the bus whose transactions its operations join, the owners of
// the rows a stop writes (the Confirmations a stop drops and a START records, and the opt-outs)
// and the Instance clock, which dates a stop recorded by hand.
func New(bus *events.Bus, confs *confirmations.Module, unsubs *unsubscribes.Module, now func() time.Time) *Module {
	return &Module{bus: bus, confirmations: confs, unsubs: unsubs, now: now}
}

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
