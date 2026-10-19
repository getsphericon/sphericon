// Package webhooks is the one place a Workspace's Webhook endpoints are created,
// updated, read, listed and deleted. It owns the URL rule, the signing secret
// (generated server-side, stored sealed, returned decrypted) and the Enterprise
// rule that selecting audit.entry needs a license (ADR 0022). The site and external
// handlers only map DTOs and the sentinels below to a 422. The endpoint is an audited
// entity, so the scoped client records its changes (a regenerated secret included); the
// one thing the module records itself is a test delivery, which writes no row.
//
// Order: a catalogue, ascending by id (see package pagination).
package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrInvalidURL: the URL is not an absolute http(s) URL (a 422 on `url`).
	ErrInvalidURL = errors.New("webhooks: url must be an absolute http or https URL")
	// ErrAuditNeedsLicense: audit.entry was selected without an Enterprise license
	// (a 422 on `eventTypes`).
	ErrAuditNeedsLicense = errors.New("webhooks: audit.entry needs an Enterprise license")
)

// Licensing reports whether the Enterprise license is active. A nil Licensing
// means unlicensed.
type Licensing interface{ Licensed() bool }

// Endpoint is a Webhook endpoint with its signing secret decrypted.
type Endpoint struct {
	*ent.WebhookEndpoint
	Secret string
}

// CreateInput is a new endpoint. A nil Enabled keeps the default (enabled).
type CreateInput struct {
	URL        string
	EventTypes []string
	Enabled    *bool
}

// UpdateInput changes only what is set: a nil field is left alone.
type UpdateInput struct {
	URL        *string
	EventTypes []string
	Enabled    *bool
}

// Module is the webhooks module. Every call receives the Workspace-scoped client.
type Module struct {
	bus    *events.Bus
	cipher *secrets.Cipher
	lic    Licensing
	client webhook.Doer
}

// New builds the module. lic may be nil (unlicensed). client is the one hardened
// outbound client, used by TestSend.
func New(bus *events.Bus, cipher *secrets.Cipher, lic Licensing, client webhook.Doer) *Module {
	return &Module{bus: bus, cipher: cipher, lic: lic, client: client}
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

// ErrInvalidEventType: the event type to send a sample of is blank or too long.
var ErrInvalidEventType = errors.New("webhooks: the event type must be 1 to 100 characters")

// RegenerateSecret replaces the signing secret at once: the old one stops verifying
// with the next delivery. The row update is audited by the scoped client
// (secret_encrypted is a sensitive field, so the entry only says it changed).
func (m *Module) RegenerateSecret(ctx context.Context, s *ent.Scoped, id int64) (Endpoint, error) {
	secret, err := generateSecret()
	if err != nil {
		return Endpoint{}, err
	}
	sealed, err := m.cipher.Encrypt([]byte(secret))
	if err != nil {
		return Endpoint{}, err
	}
	e, err := s.WebhookEndpoint().UpdateOneID(id).SetSecretEncrypted(sealed).Save(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
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
		"occurredAt":  time.Now().UTC(),
		"workspaceId": s.WorkspaceID(),
		"subject":     "Sphericon test event",
		"data":        map[string]any{"test": true},
	})
	if err != nil {
		return TestResult{}, err
	}
	rec := &statusRecorder{Doer: m.client}
	sendErr := webhook.Send(ctx, rec, e.URL, e.Secret, eventType, deliveryID, body)
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
			TargetType:  "webhook_endpoint",
			TargetID:    strconv.FormatInt(e.ID, 10),
			TargetName:  urlName(e.URL),
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

func (m *Module) open(e *ent.WebhookEndpoint) (Endpoint, error) {
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

// Create validates the URL and the event filter, generates the signing secret and
// stores it sealed.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Endpoint, error) {
	if !validURL(in.URL) {
		return Endpoint{}, ErrInvalidURL
	}
	if err := m.checkTypes(in.EventTypes); err != nil {
		return Endpoint{}, err
	}
	secret, err := generateSecret()
	if err != nil {
		return Endpoint{}, err
	}
	sealed, err := m.cipher.Encrypt([]byte(secret))
	if err != nil {
		return Endpoint{}, err
	}
	e, err := s.WebhookEndpoint().Create().
		SetURL(in.URL).
		SetSecretEncrypted(sealed).
		SetEventTypes(in.EventTypes).
		SetNillableEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// Update applies the set fields. An unknown id is ent.IsNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Endpoint, error) {
	if in.URL != nil && !validURL(*in.URL) {
		return Endpoint{}, ErrInvalidURL
	}
	if err := m.checkTypes(in.EventTypes); err != nil {
		return Endpoint{}, err
	}
	upd := s.WebhookEndpoint().UpdateOneID(id).
		SetNillableURL(in.URL).
		SetNillableEnabled(in.Enabled)
	if in.EventTypes != nil {
		upd = upd.SetEventTypes(in.EventTypes)
	}
	if in.Enabled != nil {
		// Whoever flips the switch decides now: the reason it last turned itself off no
		// longer applies, and enabling starts the failure run over (ADR 0039).
		upd = upd.ClearDisabledReason()
		if *in.Enabled {
			upd = upd.ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
				ClearLastErrorStatus().ClearLastFailureAt()
		}
	}
	e, err := upd.Save(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// Delete removes the endpoint. An unknown id is ent.IsNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return s.WebhookEndpoint().DeleteOneID(id).Exec(ctx)
}

// Get returns one endpoint. An unknown id is ent.IsNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (Endpoint, error) {
	e, err := s.WebhookEndpoint().Get(ctx, id)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// List returns one page of the Workspace's endpoints, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[Endpoint], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.WebhookEndpoint().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]Endpoint, error) {
			rows, err := s.WebhookEndpoint().Query().
				Order(ent.Asc(webhookendpoint.FieldID)).Limit(limit).Offset(offset).All(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]Endpoint, len(rows))
			for i, e := range rows {
				if out[i], err = m.open(e); err != nil {
					return nil, err
				}
			}
			return out, nil
		})
}

// generateSecret returns a Standard Webhooks signing secret: the "whsec_" prefix
// plus base64 random bytes, the format the standard-webhooks library expects.
func generateSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(b), nil
}

// validURL accepts only absolute http(s) URLs. (Network-level SSRF defenses live
// in the delivery worker, which dials the resolved IP.)
func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
