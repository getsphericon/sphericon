package validation

import (
	"errors"

	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// Kind is what a refusal says about the request, in the domain's words. The server's error handler
// gives each kind its answer on every surface (review rule 15); the core knows nothing of HTTP.
type Kind int

const (
	// KindRefused is a rule of the module that said no to the input.
	KindRefused Kind = iota
	// KindNotFound is a subject that does not exist in this Workspace.
	KindNotFound
	// KindConflict is a subject whose state does not allow the request now.
	KindConflict
	// KindForbidden is a caller whose Role in the Workspace lacks the Permission the request
	// needs (ADR 0061).
	KindForbidden
	// KindUnauthorized is a sign-in that does not hold: a wrong address or password, a
	// sign-in step that expired, a wrong code or a Passkey that does not verify at sign-in.
	KindUnauthorized
	// KindDenied is a caller who did not prove what the step asks for (the current password, a
	// fresh identity proof, a Passkey of their own) or who is not the one the subject is for.
	// It says nothing of a Role: a Role without a Permission is KindForbidden.
	KindDenied
	// KindUnverified is a User whose address is not proven yet (ADR 0030): the step waits for
	// the link sent to it.
	KindUnverified
	// KindLinkInvalid is a mailed link that is forged, expired, spent, or no longer fits its
	// subject.
	KindLinkInvalid
	// KindIncomplete is a request that lacks an input this case alone needs, which the client
	// asks for only once it is refused (a password for a link opened in another browser).
	KindIncomplete
	// KindUnknownReference is a reference the request carries that names nothing the module can
	// read: a pending sign-up that is gone, a cursor the feed never handed out. A mailed link
	// that does not stand is KindLinkInvalid.
	KindUnknownReference
	// KindAcceptancePending is a User with a required acceptance pending (ADR 0062): nothing but
	// the acceptance itself is served on their own credential until it is given.
	KindAcceptancePending
	// KindSecondFactorRequired is a User the Workspace's Two-factor requirement withholds (ADR
	// 0020): the grace ended without a Second factor.
	KindSecondFactorRequired
	// KindUnlicensed is a capability of the Enterprise Edition that the Instance's License does not
	// include (ADR 0014): the Audit log without the `audit` feature.
	KindUnlicensed
)

// RefusalError is a sentinel domain error that carries its kind and the text to show a user.
// Error() is for logs and carries the package prefix ("templates: …"), so it is never shown;
// Message is the i18n text, looked up when it is asked for because the locale is only set once
// the process boots. Callers still match with errors.Is. A handler returns it as it is: the
// server's error handler answers it by its kind.
type RefusalError struct {
	kind  Kind
	field string
	text  string
	key   i18n.ID
	// args fill the message's placeholders; parent is the refusal this one was derived from by
	// Args or On, which errors.Is still matches.
	args   map[string]any
	parent *RefusalError
}

// Args is the refusal with its message's placeholders filled by args ("at most {{.Max}}"):
// errors.Is still matches r.
func (r *RefusalError) Args(args map[string]any) *RefusalError {
	derived := *r
	derived.args, derived.parent = args, r
	return &derived
}

// On is the refusal reported against the request field named field (its json name), for a rule
// several fields share (an address under `email` or `newEmail`): errors.Is still matches r.
func (r *RefusalError) On(field string) *RefusalError {
	derived := *r
	derived.field, derived.parent = field, r
	return &derived
}

// Is reports whether target is a refusal r was derived from by Args or On.
func (r *RefusalError) Is(target error) bool {
	for p := r.parent; p != nil; p = p.parent {
		if p == target {
			return true
		}
	}
	return false
}

// Refuse makes the sentinel for a refused input (KindRefused): text is its Error() and key the
// i18n message a user is shown.
func Refuse(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindRefused, text: text, key: key}
}

// RefuseField is Refuse for a refusal of one request field, named by its json name: the answer
// lists the message under that field.
func RefuseField(field, text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindRefused, field: field, text: text, key: key}
}

// ConflictField is Conflict for a subject whose state refuses one request field, named by its json
// name: the answer lists the message under that field (an address another Contact already has).
func ConflictField(field, text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindConflict, field: field, text: text, key: key}
}

// NotFound makes the sentinel for a subject that does not exist in this Workspace (KindNotFound).
func NotFound(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindNotFound, text: text, key: key}
}

// Conflict makes the sentinel for a subject whose state does not allow the request now
// (KindConflict).
func Conflict(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindConflict, text: text, key: key}
}

// Forbidden makes the sentinel for a caller whose Role lacks the Permission the request needs
// (KindForbidden).
func Forbidden(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindForbidden, text: text, key: key}
}

// Unauthorized makes the sentinel for a sign-in that does not hold (KindUnauthorized).
func Unauthorized(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindUnauthorized, text: text, key: key}
}

// Denied makes the sentinel for a caller who did not prove what the step asks for, or who is
// not the one the subject is for (KindDenied).
func Denied(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindDenied, text: text, key: key}
}

// Unverified makes the sentinel for a User whose address is not proven yet (KindUnverified).
func Unverified(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindUnverified, text: text, key: key}
}

// LinkInvalid makes the sentinel for a mailed link that does not stand (KindLinkInvalid).
func LinkInvalid(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindLinkInvalid, text: text, key: key}
}

// Incomplete makes the sentinel for a request that lacks an input this case alone needs
// (KindIncomplete).
func Incomplete(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindIncomplete, text: text, key: key}
}

// UnknownReference makes the sentinel for a reference the request carries that names nothing the
// module can read (KindUnknownReference).
func UnknownReference(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindUnknownReference, text: text, key: key}
}

// AcceptancePending makes the sentinel for a User with a required acceptance pending
// (KindAcceptancePending).
func AcceptancePending(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindAcceptancePending, text: text, key: key}
}

// SecondFactorRequired makes the sentinel for a User the Two-factor requirement withholds
// (KindSecondFactorRequired).
func SecondFactorRequired(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindSecondFactorRequired, text: text, key: key}
}

// Unlicensed makes the sentinel for a capability the Instance's License does not include
// (KindUnlicensed).
func Unlicensed(text string, key i18n.ID) *RefusalError {
	return &RefusalError{kind: KindUnlicensed, text: text, key: key}
}

func (r *RefusalError) Error() string { return r.text }

// Kind is what the refusal says about the request.
func (r *RefusalError) Kind() Kind { return r.kind }

// Field is the json name of the request field the refusal is about, "" when it is about no one
// field. A refusal of any kind may name one.
func (r *RefusalError) Field() string { return r.field }

// Message is the refusal's text in the instance locale.
func (r *RefusalError) Message() string { return i18n.T(r.key, r.args) }

// With is the refusal for one input: the sentinel's message followed by detail, which says what
// is wrong with it. errors.Is still matches the sentinel, and errors.As finds it for its kind.
func (r *RefusalError) With(detail string) error { return &detailedError{base: r, detail: detail} }

type detailedError struct {
	base   *RefusalError
	detail string
}

func (e *detailedError) Error() string   { return e.base.text + ": " + e.detail }
func (e *detailedError) Message() string { return e.base.Message() + ": " + e.detail }
func (e *detailedError) Unwrap() error   { return e.base }

type messaged interface {
	error
	Message() string
}

// Shown is the text a user is shown for err when it is a refusal of a domain module (a
// RefusalError or a FieldsError in its chain), and false for any other error, whose text is not
// written for a user. A result that reports several items at once (a batch) words each failed item
// with it.
func Shown(err error) (string, bool) {
	if form, ok := errors.AsType[*FieldsError](err); ok {
		return form.Detail(), true
	}
	if _, ok := errors.AsType[*RefusalError](err); ok {
		return Message(err), true
	}
	return "", false
}

// Message is the text to show a user for a domain error: the message of the refusal in its
// chain, otherwise the error's own text, which is already written for the user where the
// error is not a refusal (emailrender's ErrInvalidContent carries its reason).
func Message(err error) string {
	if m, ok := errors.AsType[messaged](err); ok {
		return m.Message()
	}
	return err.Error()
}
