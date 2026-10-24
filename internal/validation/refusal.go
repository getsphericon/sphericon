package validation

import (
	"errors"

	"github.com/getsphericon/sphericon/internal/i18n"
)

// RefusalError is a sentinel domain error that carries the text to show a user. Error() is for logs
// and carries the package prefix ("templates: …"), so it is never shown; Message is the i18n
// text, looked up when it is asked for because the locale is only set once the process boots.
// Callers still match with errors.Is.
type RefusalError struct {
	text string
	key  string
}

// Refuse makes the sentinel for a refusal: text is its Error() and key the i18n message a
// handler shows.
func Refuse(text, key string) *RefusalError { return &RefusalError{text: text, key: key} }

func (r *RefusalError) Error() string { return r.text }

// Message is the refusal's text in the instance locale.
func (r *RefusalError) Message() string { return i18n.T(r.key, nil) }

// With is the refusal for one input: the sentinel's message followed by detail, which says what
// is wrong with it. errors.Is still matches the sentinel.
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
