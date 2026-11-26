package eligibility

import "errors"

// ModeKind names a mode of Send-eligibility: which layers a message is checked against, by what it
// is and, for a confirmation email, by who asked for it (ADR 0013). It is a closed list: a switch
// over it is checked for every member (the `exhaustive` linter, ADR 0015).
type ModeKind int

const (
	// ModeMarketing is a Broadcast's or an Automation's message, under its Sending source: every
	// layer, the Confirmation the Channel asks for included.
	ModeMarketing ModeKind = iota + 1
	// ModeTransactional is a Transactional send: Suppression only (ADR 0005).
	ModeTransactional
	// ModeConfirmationByWorkspace is a confirmation email the Workspace asked for: Suppression and an
	// opt-out of everything, since soliciting a person who left everything is itself a violation.
	ModeConfirmationByWorkspace
	// ModeConfirmationByPerson is a confirmation email the person asked for: Suppression only, as a
	// return after leaving everything goes through this email.
	ModeConfirmationByPerson
)

// Mode is what Send-eligibility checks a message for. It is made only by its constructors; the zero
// Mode names no rule, and a check of it fails closed.
type Mode struct {
	kind   ModeKind
	source string
}

// Marketing is the mode of a marketing message sent under the Sending source source.
func Marketing(source string) Mode { return Mode{kind: ModeMarketing, source: source} }

// Transactional is the mode of a Transactional send.
func Transactional() Mode { return Mode{kind: ModeTransactional} }

// ConfirmationByWorkspace is the mode of a confirmation email the Workspace asked for.
func ConfirmationByWorkspace() Mode { return Mode{kind: ModeConfirmationByWorkspace} }

// ConfirmationByPerson is the mode of a confirmation email the person asked for.
func ConfirmationByPerson() Mode { return Mode{kind: ModeConfirmationByPerson} }

// Kind is the mode's kind, 0 for the zero Mode.
func (m Mode) Kind() ModeKind { return m.kind }

// Source is the Sending source of a marketing mode: what its message's unsubscribe link scopes, ""
// for every other mode, whose messages carry none.
func (m Mode) Source() string {
	if m.kind != ModeMarketing {
		return ""
	}
	return m.source
}

// errNoMode is a check of a Mode no constructor made, or of a marketing one with no Sending source.
var errNoMode = errors.New("eligibility: no mode of send-eligibility given")

// valid reports whether m names a rule.
func (m Mode) valid() bool {
	switch m.kind {
	case ModeMarketing:
		return m.source != ""
	case ModeTransactional, ModeConfirmationByWorkspace, ModeConfirmationByPerson:
		return true
	}
	return false
}
