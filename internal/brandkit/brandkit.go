package brandkit

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrNotSaved means the Workspace has no saved Brand kit, so there is nothing to make starter
// Templates from.
var ErrNotSaved = errors.New("brandkit: the Brand kit is not saved yet")

// Module reads and saves the Brand kit and makes the starter Templates from it. It holds no
// Workspace: every call receives the Workspace-scoped client (ADR 0017).
//
// Anything a caller shows to an agent from a Kit (an MCP tool, a prompt) is untrusted data: the
// texts come from people of the Workspace.
type Module struct {
	bus       *events.Bus
	templates *templates.Module
}

// New builds the Brand kit module. The starter Templates are written through the templates module
// so they are ordinary Templates in every respect.
func New(bus *events.Bus, tpl *templates.Module) *Module {
	return &Module{bus: bus, templates: tpl}
}

// Input is the whole of a save: every value of the kit.
type Input struct {
	AccentColor     string
	TextColor       string
	BackgroundColor string
	HeadingFont     string
	BodyFont        string
	LogoURL         string
	ButtonTextColor string
	ButtonRadius    int
	FooterText      string
}

// Get returns the Workspace's Brand kit: the saved one, or the defaults with Saved false.
func (m *Module) Get(ctx context.Context, s *ent.Scoped) (Kit, error) {
	row, err := s.BrandKit().Query().Only(ctx)
	if ent.IsNotFound(err) {
		return Defaults(), nil
	}
	if err != nil {
		return Kit{}, err
	}
	return kitOf(row), nil
}

// Lint runs the lint of ADR 0040 over a message against the Workspace's Brand kit (none saved:
// no brand warnings). Its warnings are advice for the author; only its Error blocks.
func (m *Module) Lint(ctx context.Context, s *ent.Scoped, subject, body string) (emailrender.Report, error) {
	kit, err := m.Get(ctx, s)
	if err != nil {
		return emailrender.Report{}, err
	}
	return emailrender.Lint(subject, body, kit.Brand()), nil
}

// Save stores the Brand kit. The first save creates it and, in the same transaction, the starter
// Templates in its brand; a later save replaces the values and creates nothing. No message
// already written is read or written: the values were copied into each when it was authored.
// An invalid value is a *validation.Error: the kit's values are inlined into the CSS and HTML of the
// starter documents, so they are checked strictly.
func (m *Module) Save(ctx context.Context, s *ent.Scoped, in Input) (Kit, error) {
	in, err := validate(in)
	if err != nil {
		return Kit{}, err
	}
	// The starters of a first save are rendered here, before the transaction that writes them.
	current, err := m.Get(ctx, s)
	if err != nil {
		return Kit{}, err
	}
	var starters []renderedStarter
	if !current.Saved {
		if starters, err = m.renderStarters(ctx, kitOfInput(in)); err != nil {
			return Kit{}, err
		}
	}
	var saved *ent.BrandKit
	// A second first save that loses the race hits the unique index of the Workspace; it runs again
	// as an update, so the starters are made once.
	for attempt := 0; ; attempt++ {
		saved, err = m.save(ctx, s, in, starters)
		if ent.IsConstraintError(err) && attempt == 0 {
			continue
		}
		break
	}
	if err != nil {
		return Kit{}, err
	}
	return kitOf(saved), nil
}

func (m *Module) save(ctx context.Context, s *ent.Scoped, in Input, starters []renderedStarter) (*ent.BrandKit, error) {
	var saved *ent.BrandKit
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.BrandKit().Query().Only(ctx)
		switch {
		case ent.IsNotFound(err):
			saved, err = ts.BrandKit().Create().
				SetAccentColor(in.AccentColor).
				SetTextColor(in.TextColor).
				SetBackgroundColor(in.BackgroundColor).
				SetHeadingFont(in.HeadingFont).
				SetBodyFont(in.BodyFont).
				SetLogoURL(in.LogoURL).
				SetButtonTextColor(in.ButtonTextColor).
				SetButtonRadius(in.ButtonRadius).
				SetFooterText(in.FooterText).
				Save(ctx)
			if err != nil {
				return err
			}
			_, err = m.createStarters(ctx, ts, starters)
			return err
		case err != nil:
			return err
		}
		saved, err = ts.BrandKit().UpdateOneID(row.ID).
			SetAccentColor(in.AccentColor).
			SetTextColor(in.TextColor).
			SetBackgroundColor(in.BackgroundColor).
			SetHeadingFont(in.HeadingFont).
			SetBodyFont(in.BodyFont).
			SetLogoURL(in.LogoURL).
			SetButtonTextColor(in.ButtonTextColor).
			SetButtonRadius(in.ButtonRadius).
			SetFooterText(in.FooterText).
			Save(ctx)
		return err
	})
	return saved, err
}

// RecreateStarterTemplates makes the four starter Templates again from the saved Brand kit and
// returns them. It only adds Templates: an existing Template, Broadcast or Automation step is
// not touched. ErrNotSaved until the Brand kit has been saved.
func (m *Module) RecreateStarterTemplates(ctx context.Context, s *ent.Scoped) ([]*ent.EmailTemplate, error) {
	kit, err := m.Get(ctx, s)
	if err != nil {
		return nil, err
	}
	if !kit.Saved {
		return nil, ErrNotSaved
	}
	starters, err := m.renderStarters(ctx, kit)
	if err != nil {
		return nil, err
	}
	var made []*ent.EmailTemplate
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var err error
		made, err = m.createStarters(ctx, ts, starters)
		return err
	})
	return made, err
}

// renderedStarter is a starter with its document already rendered to the body to store.
type renderedStarter struct {
	starter  Starter
	authored emailrender.Authored
}

// renderStarters renders the four starter documents. The first render of a process starts the
// document runtime, which takes seconds, so it runs before a transaction opens: a transaction
// never holds its database connection through a render.
func (m *Module) renderStarters(ctx context.Context, kit Kit) ([]renderedStarter, error) {
	var out []renderedStarter
	for _, starter := range Starters(kit) {
		authored, err := m.templates.Prepare(ctx, &starter.Subject, starter.Doc)
		if err != nil {
			return nil, err
		}
		out = append(out, renderedStarter{starter: starter, authored: authored})
	}
	return out, nil
}

// createStarters writes rendered starters as Templates, inside the caller's transaction.
func (m *Module) createStarters(ctx context.Context, s *ent.Scoped, starters []renderedStarter) ([]*ent.EmailTemplate, error) {
	var made []*ent.EmailTemplate
	for _, r := range starters {
		tpl, err := m.templates.Create(ctx, s, templates.CreateInput{
			Name:     r.starter.Name,
			Subject:  &r.starter.Subject,
			Doc:      r.starter.Doc,
			Prepared: &r.authored,
		})
		if err != nil {
			return nil, err
		}
		made = append(made, tpl)
	}
	return made, nil
}

func kitOfInput(in Input) Kit {
	return Kit{
		Saved:           true,
		AccentColor:     in.AccentColor,
		TextColor:       in.TextColor,
		BackgroundColor: in.BackgroundColor,
		HeadingFont:     in.HeadingFont,
		BodyFont:        in.BodyFont,
		LogoURL:         in.LogoURL,
		ButtonTextColor: in.ButtonTextColor,
		ButtonRadius:    in.ButtonRadius,
		FooterText:      in.FooterText,
	}
}

func kitOf(row *ent.BrandKit) Kit {
	return Kit{
		Saved:           true,
		AccentColor:     row.AccentColor,
		TextColor:       row.TextColor,
		BackgroundColor: row.BackgroundColor,
		HeadingFont:     row.HeadingFont,
		BodyFont:        row.BodyFont,
		LogoURL:         row.LogoURL,
		ButtonTextColor: row.ButtonTextColor,
		ButtonRadius:    row.ButtonRadius,
		FooterText:      row.FooterText,
		UpdatedAt:       row.UpdatedAt,
	}
}

var (
	hexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	fontFamily = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ,-]{0,199}$`)
)

// validate checks every value and returns the input with its colors in lower case. The values go
// into style attributes and attribute values of a document, so each is held to a tight charset
// rather than escaped.
func validate(in Input) (Input, error) {
	for _, c := range []struct {
		field string
		value *string
	}{
		{"accentColor", &in.AccentColor},
		{"textColor", &in.TextColor},
		{"backgroundColor", &in.BackgroundColor},
		{"buttonTextColor", &in.ButtonTextColor},
	} {
		if !hexColor.MatchString(*c.value) {
			return in, &validation.Error{Field: c.field, Message: i18n.T("errors.brandkit_color", map[string]any{"Field": c.field})}
		}
		*c.value = strings.ToLower(*c.value)
	}
	for _, f := range []struct {
		field string
		value string
	}{{"headingFont", in.HeadingFont}, {"bodyFont", in.BodyFont}} {
		if !fontFamily.MatchString(f.value) {
			return in, &validation.Error{Field: f.field, Message: i18n.T("errors.brandkit_font", map[string]any{"Field": f.field})}
		}
	}
	if !validLogo(in.LogoURL) {
		return in, &validation.Error{Field: "logoUrl", Message: i18n.T("errors.brandkit_logo", nil)}
	}
	if !validFooter(in.FooterText) {
		return in, &validation.Error{Field: "footerText", Message: i18n.T("errors.brandkit_footer", nil)}
	}
	return in, nil
}

// validLogo accepts no logo, or an absolute http(s) URL with no character that could leave the
// attribute it is written into or start Liquid markup.
func validLogo(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "{}\"'<>\\` \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validFooter accepts plain text: no control characters and no Liquid markup (the footer is
// written into a document as a text node, which the renderer escapes, but Liquid runs over the
// whole body), within the cap.
func validFooter(text string) bool {
	if strings.Contains(text, "{{") || strings.Contains(text, "{%") {
		return false
	}
	return !strings.ContainsFunc(text, unicode.IsControl)
}
