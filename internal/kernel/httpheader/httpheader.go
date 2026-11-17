// Package httpheader is one header of an outbound HTTP call as an http Step or Action stores it
// (ADR 0039), the type the stored definitions and internal/infra/httpcall share.
package httpheader

// Header is one request header. In a stored call Value is ciphertext. In a saved call, an empty
// Value keeps the stored value of the header with the same name.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}
