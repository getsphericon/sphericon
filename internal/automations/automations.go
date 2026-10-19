// Package automations is the one place an Automation's definition is stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it; the step executor (internal/jobs) decodes the same Step format,
// so the storage JSON has a single definition.
package automations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Step kinds.
const (
	StepEmail     = "email"
	StepWait      = "wait"
	StepApplyTag  = "apply_tag"
	StepRemoveTag = "remove_tag"
	StepHTTP      = "http"
)

// Step is one node of an Automation's linear sequence, in the executor's on-disk
// JSON form. Email steps carry Subject and Body (HTML), wait steps Seconds, tag
// steps Tag (the Tag name).
type Step struct {
	Type    string `json:"type"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
	// Doc is the editor's JSON document an email step's Body was built from, and EditorVersion
	// the editor that rendered it (ADR 0040). A step written as HTML has neither. A step being
	// saved carries a Doc or a Body, not both: the save renders the Doc to the Body.
	Doc           json.RawMessage `json:"doc,omitempty"`
	EditorVersion string          `json:"editorVersion,omitempty"`
	Seconds       int             `json:"seconds,omitempty"`
	Tag           string          `json:"tag,omitempty"`

	// HTTP steps (ADR 0039) hold ID, URL, Headers, Body (a Liquid JSON template) and
	// Secret. Stored, URL, header values and Secret are ciphertext, sealed per field;
	// in a step being saved, an empty one keeps the stored value of the step with the
	// same ID, and ClearSecret drops the signing secret.
	ID          string   `json:"id,omitempty"`
	URL         string   `json:"url,omitempty"`
	Headers     []Header `json:"headers,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	ClearSecret bool     `json:"-"`
}

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Automation in this Workspace.
	ErrNotFound = errors.New("automations: automation not found")
	// ErrInvalidStep: a step has an unknown type, a negative wait, or a tag step
	// without a tag name, or an email step whose content cannot render.
	ErrInvalidStep = errors.New("automations: invalid step")

	errBodyAndDoc = errors.New("automations: a step writes a body or a doc, not both")
)

// build renders every email step that holds a document to its body, in the same save (ADR 0040,
// the Template rules): a step with a document and a body is refused, and a document that cannot
// be rendered refuses the whole save, naming the step (1-based). A step with no document is
// left as it is, so a body-only write stores no document.
func build(ctx context.Context, steps []Step) ([]Step, error) {
	out := make([]Step, len(steps))
	copy(out, steps)
	for i, s := range out {
		if s.Type != StepEmail || len(s.Doc) == 0 {
			continue
		}
		var body *string
		if s.Body != "" {
			body = &s.Body
		}
		c, err := emailrender.ResolveContent(ctx, &s.Subject, body, s.Doc, errBodyAndDoc)
		switch {
		case errors.Is(err, errBodyAndDoc):
			return nil, fmt.Errorf("%w: step %d: write a body or a doc, not both", ErrInvalidStep, i+1)
		case errors.Is(err, emailrender.ErrInvalidContent):
			return nil, fmt.Errorf("%w: step %d: %w", ErrInvalidStep, i+1, err)
		case err != nil:
			return nil, err
		}
		out[i].Body, out[i].EditorVersion = c.Authored.Body, c.Authored.EditorVersion
	}
	return out, nil
}

// Decode parses a stored definition. An empty definition has no steps.
func Decode(definition string) ([]Step, error) {
	if definition == "" {
		return nil, nil
	}
	var steps []Step
	if err := json.Unmarshal([]byte(definition), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// Encode serializes steps into the stored definition, dropping the fields that do
// not belong to each step's type, trimming tag names and sanitizing an email step's body.
func Encode(steps []Step) (string, error) {
	stored := make([]Step, 0, len(steps))
	for _, s := range steps {
		switch s.Type {
		case StepWait:
			stored = append(stored, Step{Type: StepWait, Seconds: s.Seconds})
		case StepApplyTag, StepRemoveTag:
			stored = append(stored, Step{Type: s.Type, Tag: strings.TrimSpace(s.Tag)})
		case StepHTTP:
			stored = append(stored, Step{Type: StepHTTP, ID: s.ID, URL: s.URL, Headers: s.Headers, Body: s.Body, Secret: s.Secret})
		default:
			email := Step{Type: StepEmail, Subject: s.Subject, Body: emailrender.SanitizeHTML(s.Body)}
			if len(s.Doc) > 0 {
				email.Doc, email.EditorVersion = s.Doc, s.EditorVersion
			}
			stored = append(stored, email)
		}
	}
	b, err := json.Marshal(stored)
	return string(b), err
}

func validate(steps []Step) error {
	for _, s := range steps {
		switch s.Type {
		case StepEmail:
			if err := emailrender.Lint(s.Subject, s.Body, nil).Error; err != nil {
				return fmt.Errorf("%w: %w", ErrInvalidStep, err)
			}
		case StepHTTP:
		case StepWait:
			if s.Seconds < 0 {
				return ErrInvalidStep
			}
		case StepApplyTag, StepRemoveTag:
			if strings.TrimSpace(s.Tag) == "" {
				return ErrInvalidStep
			}
		default:
			return ErrInvalidStep
		}
	}
	return nil
}

// Module is the automations module.
type Module struct{ cipher *secrets.Cipher }

// New builds the automations module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017). The cipher seals an HTTP step's secrets.
func New(cipher *secrets.Cipher) *Module { return &Module{cipher: cipher} }

// CreateInput is the data for a new Automation. It is always created as a draft:
// activating is a separate, deliberate operation.
type CreateInput struct {
	Name         string
	TriggerEvent string
	Steps        []Step
}

// UpdateInput changes an Automation; nil fields are left alone.
type UpdateInput struct {
	Name         *string
	TriggerEvent *string
	Steps        *[]Step
}

// List returns one page of the Workspace's Automations, newest first.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Automation], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Automation().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Automation, error) {
			return s.Automation().Query().Order(ent.Desc(automation.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Get returns one Automation of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	a, err := s.Automation().Get(ctx, id)
	return a, notFound(err)
}

// Create stores a new draft Automation.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Automation, error) {
	built, err := build(ctx, in.Steps)
	if err != nil {
		return nil, err
	}
	in.Steps = built
	if err := validate(in.Steps); err != nil {
		return nil, err
	}
	q := s.Automation().Create().
		SetName(in.Name).
		SetTriggerEvent(in.TriggerEvent).
		SetStatus(automation.StatusDraft)
	if in.Steps != nil {
		sealed, err := m.sealSteps(in.Steps, nil)
		if err != nil {
			return nil, err
		}
		def, err := Encode(sealed)
		if err != nil {
			return nil, err
		}
		q = q.SetDefinition(def)
	}
	return q.Save(ctx)
}

// Update changes the Automation's name, trigger or steps. The status is untouched.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Automation, error) {
	q := s.Automation().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableTriggerEvent(in.TriggerEvent)
	if in.Steps != nil {
		built, err := build(ctx, *in.Steps)
		if err != nil {
			return nil, err
		}
		in.Steps = &built
		if err := validate(*in.Steps); err != nil {
			return nil, err
		}
		cur, err := s.Automation().Get(ctx, id)
		if err != nil {
			return nil, notFound(err)
		}
		stored, err := Decode(cur.Definition)
		if err != nil {
			return nil, err
		}
		sealed, err := m.sealSteps(*in.Steps, stored)
		if err != nil {
			return nil, err
		}
		def, err := Encode(sealed)
		if err != nil {
			return nil, err
		}
		q = q.SetDefinition(def)
	}
	a, err := q.Save(ctx)
	return a, notFound(err)
}

// Delete removes the Automation.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.Automation().DeleteOneID(id).Exec(ctx)
	return notFound(err)
}

// Activate starts enrolling Contacts into the Automation.
func (m *Module) Activate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.setStatus(ctx, s, id, automation.StatusActive)
}

// Deactivate stops new enrollments; in-flight Enrollments finish.
func (m *Module) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.setStatus(ctx, s, id, automation.StatusDraft)
}

func (m *Module) setStatus(ctx context.Context, s *ent.Scoped, id int64, status automation.Status) (*ent.Automation, error) {
	a, err := s.Automation().UpdateOneID(id).
		SetStatus(status).
		Save(ctx)
	return a, notFound(err)
}

func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}
