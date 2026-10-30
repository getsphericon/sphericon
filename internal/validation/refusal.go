package validation

import (
	"errors"

	"github.com/getsphericon/sphericon/internal/i18n"
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
	key   string
}

// Refuse makes the sentinel for a refused input (KindRefused): text is its Error() and key the
// i18n message a user is shown.
func Refuse(text, key string) *RefusalError {
	return &RefusalError{kind: KindRefused, text: text, key: key}
}

// RefuseField is Refuse for a refusal of one request field, named by its json name: the answer
// lists the message under that field.
func RefuseField(field, text, key string) *RefusalError {
	return &RefusalError{kind: KindRefused, field: field, text: text, key: key}
}

// NotFound makes the sentinel for a subject that does not exist in this Workspace (KindNotFound).
func NotFound(text, key string) *RefusalError {
	return &RefusalError{kind: KindNotFound, text: text, key: key}
}

// Conflict makes the sentinel for a subject whose state does not allow the request now
// (KindConflict).
func Conflict(text, key string) *RefusalError {
	return &RefusalError{kind: KindConflict, text: text, key: key}
}

// Forbidden makes the sentinel for a caller whose Role lacks the Permission the request needs
// (KindForbidden).
func Forbidden(text, key string) *RefusalError {
	return &RefusalError{kind: KindForbidden, text: text, key: key}
}

// Unauthorized makes the sentinel for a sign-in that does not hold (KindUnauthorized).
func Unauthorized(text, key string) *RefusalError {
	return &RefusalError{kind: KindUnauthorized, text: text, key: key}
}

// Denied makes the sentinel for a caller who did not prove what the step asks for, or who is
// not the one the subject is for (KindDenied).
func Denied(text, key string) *RefusalError {
	return &RefusalError{kind: KindDenied, text: text, key: key}
}

// Unverified makes the sentinel for a User whose address is not proven yet (KindUnverified).
func Unverified(text, key string) *RefusalError {
	return &RefusalError{kind: KindUnverified, text: text, key: key}
}

// LinkInvalid makes the sentinel for a mailed link that does not stand (KindLinkInvalid).
func LinkInvalid(text, key string) *RefusalError {
	return &RefusalError{kind: KindLinkInvalid, text: text, key: key}
}

// Incomplete makes the sentinel for a request that lacks an input this case alone needs
// (KindIncomplete).
func Incomplete(text, key string) *RefusalError {
	return &RefusalError{kind: KindIncomplete, text: text, key: key}
}

func (r *RefusalError) Error() string { return r.text }

// Kind is what the refusal says about the request.
func (r *RefusalError) Kind() Kind { return r.kind }

// Field is the json name of the request field the refusal is about, "" when it is about no one
// field.
func (r *RefusalError) Field() string { return r.field }

// Message is the refusal's text in the instance locale.
func (r *RefusalError) Message() string { return i18n.T(r.key, nil) }

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

// Message is the text to show a user for a domain error: the message of the refusal in its
// chain, otherwise the error's own text, which is already written for the user where the
// error is not a refusal (emailrender's ErrInvalidContent carries its reason).
func Message(err error) string {
	if m, ok := errors.AsType[messaged](err); ok {
		return m.Message()
	}
	return err.Error()
}
