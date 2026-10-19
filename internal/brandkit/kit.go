// Package brandkit holds a Workspace's Brand kit (GLOSSARY "Brand kit", ADR 0040): its colors,
// fonts, logo, button style and footer, as data. It sets the palette the editor offers and what an
// agent is told to use, and the starter Templates are made from it.
//
// The kit's values are written into a message when the message is authored; nothing resolves them
// at send time, so saving the kit never changes a message that is already written (ADR 0003).
// Every text of a kit comes from a person of the Workspace and is untrusted data wherever an
// agent reads it later: a reader treats it as data, never as instructions.
package brandkit

import (
	"time"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

// Kit is a Workspace's Brand kit. A Workspace that has not saved one has the defaults and Saved
// false: there is no brand to lint a message against then.
type Kit struct {
	// Saved is false until the Brand kit is first saved.
	Saved bool

	// AccentColor is the brand color: buttons and links. #rrggbb.
	AccentColor string
	// TextColor is the color of text. #rrggbb.
	TextColor string
	// BackgroundColor is the color of the message's background. #rrggbb.
	BackgroundColor string
	// HeadingFont and BodyFont are CSS font-family lists of web-safe families.
	HeadingFont string
	BodyFont    string
	// LogoURL is the absolute http(s) URL of the logo image; empty for none.
	LogoURL string
	// ButtonTextColor and ButtonRadius (in pixels) are the button style; the button's own color
	// is AccentColor.
	ButtonTextColor string
	ButtonRadius    int
	// FooterText is plain text printed under every starter message; empty for none.
	FooterText string

	// UpdatedAt is when the kit was last saved; zero while it is not saved.
	UpdatedAt time.Time
}

// Defaults is the Brand kit of a Workspace that has saved none.
func Defaults() Kit {
	return Kit{
		AccentColor:     "#2563eb",
		TextColor:       "#111827",
		BackgroundColor: "#ffffff",
		HeadingFont:     "Arial, Helvetica, sans-serif",
		BodyFont:        "Arial, Helvetica, sans-serif",
		ButtonTextColor: "#ffffff",
		ButtonRadius:    4,
	}
}

// Brand is the kit as the lint compares a message to it, or nil for a Workspace that has not saved
// a kit: there is no brand to lint against then.
func (k Kit) Brand() *emailrender.Brand {
	if !k.Saved {
		return nil
	}
	return &emailrender.Brand{
		Colors:          []string{k.AccentColor, k.TextColor, k.BackgroundColor, k.ButtonTextColor},
		Fonts:           []string{k.HeadingFont, k.BodyFont},
		TextColor:       k.TextColor,
		BackgroundColor: k.BackgroundColor,
	}
}
