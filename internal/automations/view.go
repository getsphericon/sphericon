package automations

import (
	"encoding/json"

	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

// StepView is a stored Step as the API shows it: only the fields of its kind are set, so a field
// another kind carries is absent rather than empty. An HTTP step is masked: its URL, header values
// and signing secret never leave the server, only the header names and whether a secret is set.
type StepView struct {
	Type          automationsteps.Kind
	Subject, Body *string
	// Text is a Telegram or an SMS step's message; ImageURL, Buttons, LinkPreview and
	// UnsubscribeButton the rest of a Telegram step's content (LinkPreview is set, true by default,
	// on a Telegram step only).
	Text              *string
	ImageURL          *string
	Buttons           []telegramrender.Button
	LinkPreview       *bool
	UnsubscribeButton *string
	// LinkTracking is set, true by default, on an SMS step only: whether its links go out as short
	// tracked links.
	LinkTracking *bool
	// Doc is an email step's editor document, and EditorVersion the editor that rendered it; a
	// step written as HTML has neither.
	Doc           json.RawMessage
	EditorVersion *string
	Seconds       *int
	Tag           *string
	ID            *string
	// Headers is set, possibly empty, on an HTTP step only.
	Headers   []HeaderName
	HasSecret *bool
	// IntegrationID is a send step's Integration, nil for its Channel's default.
	IntegrationID *int64
}

// HeaderName is a header of an HTTP step without its value.
type HeaderName struct {
	Name string
}

// Views decodes a stored definition into the views of its steps. A definition that does not
// decode shows no steps rather than an error.
func Views(definition string) []StepView {
	stored, err := automationsteps.Decode(definition)
	if err != nil {
		return nil
	}
	views := make([]StepView, 0, len(stored))
	for i := range stored {
		views = append(views, viewOf(&stored[i]))
	}
	return views
}

func viewOf(s *automationsteps.Step) StepView {
	v := StepView{Type: s.Type}
	switch s.Type {
	case automationsteps.Wait:
		v.Seconds = &s.Seconds
	case automationsteps.ApplyTag, automationsteps.RemoveTag:
		v.Tag = &s.Tag
	case automationsteps.HTTP:
		v.ID = &s.ID
		v.Body = &s.Body
		v.HasSecret = new(s.Secret != "")
		v.Headers = make([]HeaderName, len(s.Headers))
		for i, h := range s.Headers {
			v.Headers[i] = HeaderName{Name: h.Name}
		}
	case automationsteps.Telegram:
		v.IntegrationID = s.IntegrationID
		v.Text = &s.Text
		if s.ImageURL != "" {
			v.ImageURL = &s.ImageURL
		}
		v.Buttons = append([]telegramrender.Button{}, s.Buttons...)
		v.LinkPreview = new(s.LinkPreview == nil || *s.LinkPreview)
		if s.UnsubscribeButton != "" {
			v.UnsubscribeButton = &s.UnsubscribeButton
		}
	case automationsteps.SMS:
		v.IntegrationID = s.IntegrationID
		v.Text = &s.Text
		v.LinkTracking = new(s.TracksLinks())
	case automationsteps.Email:
		v.IntegrationID = s.IntegrationID
		v.Subject = &s.Subject
		v.Body = &s.Body
		if len(s.Doc) > 0 {
			v.Doc = s.Doc
			v.EditorVersion = &s.EditorVersion
		}
	}
	return v
}
