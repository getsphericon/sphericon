package automations

import (
	"encoding/json"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

// PathView is a stored path as the API shows it: its id and a view of each of its Steps, in order.
type PathView struct {
	ID    string
	Steps []StepView
}

// StepView is a stored Step as the API shows it, one type per kind (EmailView, TelegramView,
// SMSView, WaitView, TagView, HTTPView, EndView): each holds its kind's fields only, the defaults it was
// stored without filled in. An HTTP step is masked: its URL, header values and signing secret
// never leave the server, only the header names and whether a secret is set.
type StepView interface{ stepView() }

// EmailView is an email step. Doc is its editor document and EditorVersion the editor that
// rendered it; a step written as HTML has neither. IntegrationID is nil for the Channel's default.
type EmailView struct {
	ID            string
	Type          automationstep.Kind
	Subject, Body string
	Doc           json.RawMessage
	EditorVersion *string
	IntegrationID *int64
}

// TelegramView is a Telegram step: its message, its image and unsubscribe button (nil for none),
// its buttons and whether a link unfolds a preview.
type TelegramView struct {
	ID                string
	Type              automationstep.Kind
	Text              string
	ImageURL          *string
	Buttons           []telegramrender.Button
	LinkPreview       bool
	UnsubscribeButton *string
	IntegrationID     *int64
}

// SMSView is an SMS step: its text and whether its links go out as short tracked links.
type SMSView struct {
	ID            string
	Type          automationstep.Kind
	Text          string
	LinkTracking  bool
	IntegrationID *int64
}

// WaitView is a wait step.
type WaitView struct {
	ID      string
	Type    automationstep.Kind
	Seconds int
}

// TagView is an apply tag or a remove tag step, as Type says.
type TagView struct {
	ID   string
	Type automationstep.Kind
	Tag  string
}

// HTTPView is an HTTP step without its secrets.
type HTTPView struct {
	ID        string
	Type      automationstep.Kind
	Body      string
	Headers   []HeaderName
	HasSecret bool
}

// EndView is an End: its id and kind, nothing else (ADR 0124).
type EndView struct {
	ID   string
	Type automationstep.Kind
}

func (EmailView) stepView()    {}
func (TelegramView) stepView() {}
func (SMSView) stepView()      {}
func (WaitView) stepView()     {}
func (TagView) stepView()      {}
func (HTTPView) stepView()     {}
func (EndView) stepView()      {}

// HeaderName is a header of an HTTP step without its value.
type HeaderName struct {
	Name string
}

// View is the view of a version's steps.
func View(stored automationstep.Path) PathView {
	views := make([]StepView, 0, len(stored.Steps))
	for _, st := range automationstep.All(stored) {
		views = append(views, viewOf(st))
	}
	return PathView{ID: stored.ID, Steps: views}
}

func viewOf(st *automationstep.Step) StepView {
	switch s := st.Payload.(type) {
	case automationstep.Wait:
		return WaitView{ID: st.ID, Type: st.Kind(), Seconds: s.Seconds}
	case automationstep.End:
		return EndView{ID: st.ID, Type: st.Kind()}
	case automationstep.ApplyTag:
		return TagView{ID: st.ID, Type: st.Kind(), Tag: s.Tag}
	case automationstep.RemoveTag:
		return TagView{ID: st.ID, Type: st.Kind(), Tag: s.Tag}
	case automationstep.HTTP:
		v := HTTPView{ID: st.ID, Type: st.Kind(), Body: s.Body, HasSecret: s.Secret != "", Headers: make([]HeaderName, len(s.Headers))}
		for i, h := range s.Headers {
			v.Headers[i] = HeaderName{Name: h.Name}
		}
		return v
	case automationstep.Telegram:
		v := TelegramView{
			ID: st.ID, Type: st.Kind(), IntegrationID: s.IntegrationID, Text: s.Text,
			Buttons:     append([]telegramrender.Button{}, s.Buttons...),
			LinkPreview: s.LinkPreview == nil || *s.LinkPreview,
		}
		if s.ImageURL != "" {
			v.ImageURL = &s.ImageURL
		}
		if s.UnsubscribeButton != "" {
			v.UnsubscribeButton = &s.UnsubscribeButton
		}
		return v
	case automationstep.SMS:
		return SMSView{ID: st.ID, Type: st.Kind(), IntegrationID: s.IntegrationID, Text: s.Text, LinkTracking: s.TracksLinks()}
	case automationstep.Email:
		v := EmailView{ID: st.ID, Type: st.Kind(), IntegrationID: s.IntegrationID, Subject: s.Subject, Body: s.Body}
		if len(s.Doc) > 0 {
			v.Doc, v.EditorVersion = s.Doc, &s.EditorVersion
		}
		return v
	default:
		return nil
	}
}
