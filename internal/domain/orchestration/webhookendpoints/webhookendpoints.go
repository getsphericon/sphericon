// Package webhookendpoints is the one place a Workspace's Webhook endpoints are created,
// updated, read, listed and deleted. It owns the URL rule, the signing secret
// (generated server-side, stored sealed, returned decrypted) and the Enterprise
// rule that selecting audit.entry needs a license (ADR 0022). The site and external
// handlers only map DTOs and the sentinels below to a 422. The endpoint is an audited
// entity, so the scoped client records its changes (a regenerated secret included); the
// one thing the module records itself is a test delivery, which writes no row.
//
// Order: a catalogue, ascending by id (see package pagination).
package webhookendpoints

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/ent/webhookendpointhealth"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrInvalidURL: the URL is not an absolute http(s) URL (a 422 on `url`).
	ErrInvalidURL = validation.RefuseField(fieldname.URL, "webhooks: url must be an absolute http or https URL", i18n.ErrorsWebhookUrlInvalid)
	// ErrAuditNeedsLicense: audit.entry was selected without an Enterprise license
	// (a 422 on `eventTypes`).
	ErrAuditNeedsLicense = validation.RefuseField(fieldname.EventTypes, "webhooks: audit.entry needs an Enterprise license", i18n.ErrorsWebhookAuditNeedsLicense)
	// ErrNotFound: no such endpoint in this Workspace.
	ErrNotFound = validation.NotFound("webhooks: webhook not found", i18n.ErrorsWebhookNotFound)
	// ErrRevisionChanged: the update was made on a revision the endpoint has moved past (ADR 0119):
	// someone wrote it since the client read it, or the delivery worker turned it off. Nothing is
	// written.
	ErrRevisionChanged = validation.RevisionChanged("webhooks: webhook changed since it was read", i18n.ErrorsWebhookRevisionChanged)
)

// Licensing reports whether the Enterprise license is active. A nil Licensing
// means unlicensed.
type Licensing interface{ Licensed() bool }

// Endpoint is a Webhook endpoint with its signing secret decrypted. The endpoint is a field, not
// embedded: an embedded entity would promote its client's operations onto this value.
type Endpoint struct {
	WebhookEndpoint *ent.WebhookEndpoint
	Secret          string
}

// CreateInput is a new endpoint. A nil Enabled keeps the default (enabled).
type CreateInput struct {
	URL        string
	EventTypes []string
	Enabled    *bool
}

// UpdateInput is a patch of an endpoint: a field that keeps is left alone. None of the fields can
// be cleared; an empty EventTypes receives every event.
type UpdateInput struct {
	// Revision is the endpoint's revision the edit was made on (ADR 0119).
	Revision   int64
	URL        patch.Field[string]
	EventTypes patch.Field[[]string]
	Enabled    patch.Field[bool]
}

// Module is the webhooks module. Every call receives the Workspace-scoped client.
type Module struct {
	bus    *events.Bus
	cipher *secrets.Cipher
	lic    Licensing
	client webhook.Doer
	// now is the Instance clock a test event and a disabled endpoint are dated by.
	now func() time.Time
}

// New builds the module. lic may be nil (unlicensed). client is the one hardened
// outbound client, used by TestSend. now is the Instance clock.
func New(bus *events.Bus, cipher *secrets.Cipher, lic Licensing, client webhook.Doer, now func() time.Time) *Module {
	return &Module{bus: bus, cipher: cipher, lic: lic, client: client, now: now}
}

// TestSendTimeout bounds a test delivery, the same budget as a real one.
const TestSendTimeout = 15 * time.Second

// TestResult is the outcome of one test delivery: Delivered with the receiver's Status
// on a 2xx, otherwise the closed error Kind (and the Status of an http_status failure).
type TestResult struct {
	Delivered bool
	Status    int
	Kind      webhook.ErrorKind
}

// ErrInvalidEventType means the event type to send a sample of is blank or too long.
var ErrInvalidEventType = validation.RefuseField(fieldname.EventType, "webhooks: the event type must be 1 to 100 characters", i18n.ErrorsWebhookEventTypeInvalid)

// RegenerateSecret replaces the signing secret at once: the old one stops verifying
// with the next delivery. The row update is audited by the scoped client
// (secret_encrypted is a sensitive field, so the entry only says it changed).
func (m *Module) RegenerateSecret(ctx context.Context, s *ent.Scoped, id int64) (Endpoint, error) {
	sealed, err := m.cipher.Encrypt([]byte(generateSecret()))
	if err != nil {
		return Endpoint{}, err
	}
	e, err := s.WebhookEndpoint().UpdateOneID(id).SetSecretEncrypted(sealed).Save(ctx)
	if err != nil {
		return Endpoint{}, missing(err)
	}
	return m.open(ctx, s, e)
}

// TestSend delivers a sample event of the given type to the endpoint, signed as a real
// delivery, and returns what the receiver answered. It is synchronous, works on a
// disabled endpoint and never touches the failure state. It writes no row, so it is
// recorded as an explicit Audit entry (webhook_endpoint.test).
func (m *Module) TestSend(ctx context.Context, s *ent.Scoped, id int64, eventType string) (TestResult, error) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || len(eventType) > 100 {
		return TestResult{}, ErrInvalidEventType
	}
	e, err := m.Get(ctx, s, id)
	if err != nil {
		return TestResult{}, err
	}
	deliveryID := "evt_test_" + strings.ToLower(rand.Text())
	body, err := json.Marshal(map[string]any{
		"id":          deliveryID,
		"type":        eventType,
		"occurredAt":  m.now().UTC(),
		"workspaceId": s.WorkspaceID(),
		"subject":     "Sphericon test event",
		"data":        map[string]any{"test": true},
	})
	if err != nil {
		return TestResult{}, err
	}
	rec := &statusRecorder{Doer: m.client}
	sendErr := webhook.Send(ctx, rec, e.WebhookEndpoint.URL, e.Secret, eventType, deliveryID, body, m.now())
	var res TestResult
	var delivery *webhook.DeliveryError
	switch {
	case sendErr == nil:
		res = TestResult{Delivered: true, Status: rec.status}
	case errors.As(sendErr, &delivery):
		res = TestResult{Kind: delivery.Kind, Status: delivery.Status}
	default:
		return TestResult{}, sendErr // an unusable secret, not a delivery outcome
	}
	if err := m.recordTest(ctx, s, e, eventType, res); err != nil {
		return TestResult{}, err
	}
	return res, nil
}

func (m *Module) recordTest(ctx context.Context, s *ent.Scoped, e Endpoint, eventType string, res TestResult) error {
	if k := s.Actor().Kind; k == "" || k == events.ActorIngest {
		return nil
	}
	diff := map[string]any{"eventType": eventType, "delivered": res.Delivered}
	if res.Status != 0 {
		diff["status"] = res.Status
	}
	if res.Kind != "" {
		diff["kind"] = string(res.Kind)
	}
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ts.WorkspaceID(),
			Actor:       s.Actor(),
			Action:      events.ActionWebhookEndpointTest,
			TargetType:  webhookendpoint.Label,
			TargetID:    publicid.Encode(e.WebhookEndpoint.ID),
			TargetName:  urlName(e.WebhookEndpoint.URL),
			Diff:        diff,
		})
	})
}

// urlName is the endpoint's URL without query or userinfo, the form the Audit log names it by.
func urlName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// statusRecorder remembers the status of the last response so a 2xx can be reported.
type statusRecorder struct {
	webhook.Doer

	status int
}

func (r *statusRecorder) Do(req *http.Request) (*http.Response, error) {
	resp, err := r.Doer.Do(req)
	if resp != nil {
		r.status = resp.StatusCode
	}
	return resp, err
}

// open is the Endpoint of e with its secret and its health (HealthOf), read from s when e was read
// without it.
func (m *Module) open(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint) (Endpoint, error) {
	if e.Edges.Health == nil {
		health, err := s.WebhookEndpointHealth().Query().Where(webhookendpointhealth.WebhookEndpointID(e.ID)).Only(ctx)
		if err != nil {
			return Endpoint{}, err
		}
		e.Edges.Health = health
	}
	secret, err := m.cipher.Decrypt(e.SecretEncrypted)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{WebhookEndpoint: e, Secret: string(secret)}, nil
}

func (m *Module) checkTypes(types []string) error {
	if slices.Contains(types, events.NameAuditEntry) && (m.lic == nil || !m.lic.Licensed()) {
		return ErrAuditNeedsLicense
	}
	return nil
}

// The unique constraint of an endpoint's health (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("webhook_endpoint_health_webhook_endpoint_id_key", "a health row is made once, with its endpoint (Create)"),
)

// Create validates the URL and the event filter, generates the signing secret and
// stores it sealed, with the endpoint's health beside it (ADR 0168).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Endpoint, error) {
	if !validURL(in.URL) {
		return Endpoint{}, ErrInvalidURL
	}
	if err := m.checkTypes(in.EventTypes); err != nil {
		return Endpoint{}, err
	}
	sealed, err := m.cipher.Encrypt([]byte(generateSecret()))
	if err != nil {
		return Endpoint{}, err
	}
	var e *ent.WebhookEndpoint
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		create := ts.WebhookEndpoint().Create().
			SetURL(in.URL).
			SetSecretEncrypted(sealed).
			SetEventTypes(in.EventTypes)
		if in.Enabled != nil && !*in.Enabled {
			create = create.SetDisabledAt(m.now()).SetDisabledReason(webhookendpoint.DisabledReasonOwner)
		}
		var err error
		if e, err = create.Save(ctx); err != nil {
			return err
		}
		e.Edges.Health, err = ts.WebhookEndpointHealth().Create().SetWebhookEndpointID(e.ID).Save(ctx)
		return err
	})
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(ctx, s, e)
}

// Update applies the set fields, conditional on the revision the edit was made on (ADR 0119): an
// endpoint that moved past it is ErrRevisionChanged. An unknown id is ErrNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Endpoint, error) {
	if u, ok := in.URL.Get(); ok && !validURL(u) {
		return Endpoint{}, ErrInvalidURL
	}
	types, _ := in.EventTypes.Get()
	if err := m.checkTypes(types); err != nil {
		return Endpoint{}, err
	}
	build := func() *ent.WebhookEndpointScopedUpdateOne {
		upd := s.WebhookEndpoint().EditAt(id, in.Revision).Refusing(ErrNotFound, ErrRevisionChanged)
		in.URL.ApplySet(upd.SetURL)
		in.EventTypes.ApplySet(upd.SetEventTypes)
		return upd
	}
	enabled, toggles := in.Enabled.Get()
	if !toggles {
		e, err := build().Save(ctx)
		if err != nil {
			return Endpoint{}, err
		}
		return m.open(ctx, s, e)
	}
	return m.updateHealth(ctx, s, id, in, enabled)
}

// Delete removes the endpoint. An unknown id is ErrNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return missing(s.WebhookEndpoint().DeleteOneID(id).Exec(ctx))
}

// missing turns the store's miss into ErrNotFound.
func missing(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Get returns one endpoint. An unknown id is ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (Endpoint, error) {
	e, err := s.WebhookEndpoint().Query().Where(webhookendpoint.ID(id)).WithHealth().Only(ctx)
	if err != nil {
		return Endpoint{}, missing(err)
	}
	return m.open(ctx, s, e)
}

// List returns one page of the Workspace's endpoints, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[Endpoint], error) {
	page, err := pagination.List(ctx, p, s.WebhookEndpoint().Query().WithHealth().Order(ent.Asc(webhookendpoint.FieldID)))
	if err != nil {
		return pagination.Page[Endpoint]{}, err
	}
	return page.MapErr(func(e *ent.WebhookEndpoint) (Endpoint, error) { return m.open(ctx, s, e) })
}

// generateSecret returns a Standard Webhooks signing secret: the "whsec_" prefix
// plus base64 random bytes, the format the standard-webhooks library expects. It returns no
// error: crypto/rand.Read never fails (the runtime aborts if the system source does).
func generateSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + base64.StdEncoding.EncodeToString(b)
}

// validURL accepts only absolute http(s) URLs. (Network-level SSRF defenses live
// in the delivery worker, which dials the resolved IP.)
func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
