// Package smsrender renders the text of an SMS (ADR 0078): Liquid merge tags with email's bindings,
// their values written as they are, since a text has no markup to escape. Nothing is appended.
// Tracked links are layered on by the Outbound send, not here.
package smsrender

import "github.com/getsphericon/sphericon/internal/kernel/emailrender"

// Render renders text for one recipient. An error is a template that does not render: the message
// must not go out with its tags unrendered.
func Render(text string, bindings map[string]any) (string, error) {
	out, err := emailrender.Render(text, bindings)
	if err != nil {
		return "", err
	}
	return out, nil
}

// Check refuses a text whose merge tags do not render, with no values bound: what a save of the
// text refuses.
func Check(text string) error {
	_, err := Render(text, map[string]any{})
	return err
}
