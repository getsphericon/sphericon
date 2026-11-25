package automations

import (
	"encoding/json"
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

// PathView is a stored path as the API shows it: its id and a view of each of its Steps, in order.
type PathView struct {
	ID    automationstep.ID
	Steps []StepView
}

// StepView is a stored Step as the API shows it, one type per kind (EmailView, TelegramView,
// SMSView, WaitView, TagView, HTTPView, EndView, BranchView, SplitView): each holds its kind's fields only, the defaults it was
// stored without filled in. An HTTP step is masked: its URL, header values and signing secret
// never leave the server, only the header names and whether a secret is set.
type StepView interface{ stepView() }

// EmailView is an email step. Doc is its editor document and EditorVersion the editor that
// rendered it; a step written as HTML has neither. IntegrationID is nil for the Channel's default.
type EmailView struct {
	ID            automationstep.ID
	Type          automationstep.Kind
	Subject, Body string
	Doc           json.RawMessage
	EditorVersion *string
	IntegrationID *int64
}

// TelegramView is a Telegram step: its message, its image and unsubscribe button (nil for none),
// its buttons and whether a link unfolds a preview.
type TelegramView struct {
	ID                automationstep.ID
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
	ID            automationstep.ID
	Type          automationstep.Kind
	Text          string
	LinkTracking  bool
	IntegrationID *int64
}

// WaitView is a wait step.
type WaitView struct {
	ID      automationstep.ID
	Type    automationstep.Kind
	Seconds int
}

// TagView is an apply tag or a remove tag step, as Type says.
type TagView struct {
	ID   automationstep.ID
	Type automationstep.Kind
	Tag  string
}

// HTTPView is an HTTP step without its secrets.
type HTTPView struct {
	ID        automationstep.ID
	Type      automationstep.Kind
	Body      string
	Headers   []HeaderName
	HasSecret bool
}

// EndView is an End: its id and kind, nothing else (ADR 0124).
type EndView struct {
	ID   automationstep.ID
	Type automationstep.Kind
}

// BranchView is a Branch: its listed paths, each with its Condition, its otherwise and its unknown
// path, nil for none; each path's Steps are views too.
type BranchView struct {
	ID        automationstep.ID
	Type      automationstep.Kind
	Paths     []BranchPathView
	Otherwise PathView
	Unknown   *PathView
}

// BranchPathView is a listed path of a Branch: its id, its Condition and its Steps.
type BranchPathView struct {
	ID        automationstep.ID
	Condition conditiontree.Branch
	Steps     []StepView
}

// SplitView is a Split: its paths in order, each with its weight; each path's Steps are views too.
type SplitView struct {
	ID    automationstep.ID
	Type  automationstep.Kind
	Paths []SplitPathView
}

// SplitPathView is a path of a Split: its id, its weight and its Steps.
type SplitPathView struct {
	ID     automationstep.ID
	Weight int
	Steps  []StepView
}

func (EmailView) stepView()    {}
func (BranchView) stepView()   {}
func (SplitView) stepView()    {}
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

// View is the view of a version's steps, the tree kept: a fork's view holds the views of its
// paths. A Step of a kind with no view fails it: it is never shown as an empty Step.
func View(stored automationstep.Path) (PathView, error) {
	views := make([]StepView, 0, len(stored.Steps))
	for st := range automationstep.Own(stored) {
		v, err := viewOf(st)
		if err != nil {
			return PathView{}, err
		}
		views = append(views, v)
	}
	return PathView{ID: stored.ID, Steps: views}, nil
}

// branchView is the view of a Branch, each of its paths viewed (View).
func branchView(id automationstep.ID, b automationstep.Branch) (StepView, error) {
	v := BranchView{ID: id, Type: automationstep.KindBranch, Paths: make([]BranchPathView, len(b.Paths))}
	for i, p := range b.Paths {
		steps, err := View(automationstep.Path{ID: p.ID, Steps: p.Steps})
		if err != nil {
			return nil, err
		}
		v.Paths[i] = BranchPathView{ID: p.ID, Condition: p.Condition, Steps: steps.Steps}
	}
	otherwise, err := View(b.Otherwise)
	if err != nil {
		return nil, err
	}
	v.Otherwise = otherwise
	if b.Unknown != nil {
		unknown, err := View(*b.Unknown)
		if err != nil {
			return nil, err
		}
		v.Unknown = &unknown
	}
	return v, nil
}

// splitView is the view of a Split, each of its paths viewed (View).
func splitView(id automationstep.ID, sp automationstep.Split) (StepView, error) {
	v := SplitView{ID: id, Type: automationstep.KindSplit, Paths: make([]SplitPathView, len(sp.Paths))}
	for i, p := range sp.Paths {
		steps, err := View(automationstep.Path{ID: p.ID, Steps: p.Steps})
		if err != nil {
			return nil, err
		}
		v.Paths[i] = SplitPathView{ID: p.ID, Weight: p.Weight, Steps: steps.Steps}
	}
	return v, nil
}

// viewOf is the view of the Step; a kind with none is an error.
func viewOf(st *automationstep.Step) (StepView, error) {
	switch p := st.Payload.(type) {
	case automationstep.Branch:
		return branchView(st.ID, p)
	case automationstep.Split:
		return splitView(st.ID, p)
	}
	if v := leafView(st); v != nil {
		return v, nil
	}
	return nil, fmt.Errorf("automations: step %s of kind %q has no view", st.ID, st.Kind())
}

// leafView is the view of a Step that holds no path, nil for a kind with none.
func leafView(st *automationstep.Step) StepView {
	switch s := st.Payload.(type) {
	case automationstep.Wait:
		return WaitView{ID: st.ID, Type: st.Kind(), Seconds: s.Seconds}
	case automationstep.End:
		return EndView{ID: st.ID, Type: st.Kind()}
	case automationstep.Tagging:
		return TagView{ID: st.ID, Type: st.Kind(), Tag: s.TagName()}
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
