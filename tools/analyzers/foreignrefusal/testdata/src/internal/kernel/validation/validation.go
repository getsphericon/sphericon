package validation

type RefusalError struct{ text string }

func (r *RefusalError) Error() string { return r.text }

func Refuse(text string) *RefusalError { return &RefusalError{text: text} }

// ErrShared is a refusal of the kernel's own rules: another package calls the rule's function
// instead of naming it.
var ErrShared = Refuse("validation: shared")
