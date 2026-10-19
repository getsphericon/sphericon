//go:build e2e

package e2e

// HTML wraps text in the minimal HTML body (the product's one body format).
func HTML(text string) string {
	return "<table role=\"presentation\"><tbody><tr><td><p>" + text + "</p></td></tr></tbody></table>"
}
