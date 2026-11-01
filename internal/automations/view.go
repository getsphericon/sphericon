package automations

import "encoding/json"

// StepView is a stored Step as the API shows it: only the fields of its kind are set, so a field
// another kind carries is absent rather than empty. An HTTP step is masked: its URL, header values
// and signing secret never leave the server, only the header names and whether a secret is set.
type StepView struct {
	Type          StepType
	Subject, Body *string
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
}

// HeaderName is a header of an HTTP step without its value.
type HeaderName struct {
	Name string
}

// Views decodes a stored definition into the views of its steps. A definition that does not
// decode shows no steps rather than an error.
func Views(definition string) []StepView {
	stored, err := Decode(definition)
	if err != nil {
		return nil
	}
	views := make([]StepView, 0, len(stored))
	for i := range stored {
		views = append(views, viewOf(&stored[i]))
	}
	return views
}

func viewOf(s *Step) StepView {
	v := StepView{Type: s.Type}
	switch s.Type {
	case StepWait:
		v.Seconds = &s.Seconds
	case StepApplyTag, StepRemoveTag:
		v.Tag = &s.Tag
	case StepHTTP:
		v.ID = &s.ID
		v.Body = &s.Body
		v.HasSecret = new(s.Secret != "")
		v.Headers = make([]HeaderName, len(s.Headers))
		for i, h := range s.Headers {
			v.Headers[i] = HeaderName{Name: h.Name}
		}
	case StepEmail:
		v.Subject = &s.Subject
		v.Body = &s.Body
		if len(s.Doc) > 0 {
			v.Doc = s.Doc
			v.EditorVersion = &s.EditorVersion
		}
	}
	return v
}
