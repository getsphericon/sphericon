package validation

// Error is a refused input of a domain module: Field names the request field and Message is what
// to show for it, in the instance locale. The server's error handler answers it with a 422 that
// carries the field, on every surface; a handler returns it as it is.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string { return e.Message }
