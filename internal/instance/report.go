package instance

import (
	"context"
	"fmt"
	"github.com/getsphericon/sphericon/internal/accounts"
	"runtime"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// schemaVersion is the version of the Telemetry report schema the builder implements
// (typespec/telemetry). A change to the schema bumps it in the same commit.
const schemaVersion = 1

// reportWindow is the look-back of every "in the last 30 days" figure.
const reportWindow = 30 * 24 * time.Hour

// Host is what the report says about the running binary and its surroundings. The builder
// never reads the process (build info, environment, files): the caller hands these in, so a
// test builds the same report every time.
type Host struct {
	AppVersion  string
	Edition     telemetryapi.TelemetryEdition
	DeployShape telemetryapi.TelemetryDeployShape
	OS          string
	Arch        string
	Locale      telemetryapi.TelemetryLocale
	// LicenseExpiredAt is when the license key expired; zero unless it has lapsed (ADR 0035).
	LicenseExpiredAt time.Time
}

// HostOf reads the Host of this process: the version it was built as, the edition its
// license key switches on (a bad key fails boot, so a key that is set is a valid one, and a
// lapsed one, whose expiry the caller passes, runs the core: ADR 0035), where it runs and the
// configured locale.
func HostOf(cfg *config.Config, version string, licenseExpiredAt time.Time) Host {
	edition := telemetryapi.TelemetryEditionCore
	if cfg.LicenseKey != "" && licenseExpiredAt.IsZero() {
		edition = telemetryapi.TelemetryEditionEe
	}
	return Host{
		AppVersion:  version,
		Edition:     edition,
		DeployShape: DeployShapeOf(upgrade.DetectContainer()),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Locale:      telemetryapi.TelemetryLocaleEn,

		LicenseExpiredAt: licenseExpiredAt,
	}
}

// DeployShapeOf maps what upgrade.DetectContainer found ("helm" on Kubernetes, "docker" in
// a container, "" otherwise) to the schema's deploy shape.
func DeployShapeOf(container string) telemetryapi.TelemetryDeployShape {
	switch container {
	case "helm":
		return telemetryapi.TelemetryDeployShapeKubernetes
	case "docker":
		return telemetryapi.TelemetryDeployShapeContainer
	case "":
		return telemetryapi.TelemetryDeployShapeBinary
	}
	return telemetryapi.TelemetryDeployShapeUnknown
}

// BuildReport assembles the Telemetry report (ADR 0032) from core tables only: never from an
// Enterprise Usage snapshot, which a plain install does not have. It is the one builder, the
// preview and the daily sender both call it. Counts leave it only as orders of magnitude, so
// an exact number never does, and it reads nothing that names a Workspace or a person.
func BuildReport(ctx context.Context, client *ent.Client, host Host, now time.Time) (telemetryapi.TelemetryReport, error) {
	row, err := ensureRow(ctx, client)
	if err != nil {
		return telemetryapi.TelemetryReport{}, err
	}
	since := now.Add(-reportWindow)
	var q reportQueries
	workspaces := q.count(client.Workspace.Query().Where(accounts.Live()).Count(ctx))
	// Erasure deletes the Contact row (GLOSSARY: there is no softer delete), so an erased
	// Contact is already gone, and those of a Workspace in its deletion window are left out; anonymous tracker Contacts have no email and are left out.
	contacts := q.count(client.Contact.Query().
		Where(contact.EmailNotNil(), contact.EmailNEQ(""), contact.HasWorkspaceWith(accounts.Live())).Count(ctx))
	emails := q.count(client.OutboundMessage.Query().
		Where(outboundmessage.StatusEQ(outboundmessage.StatusSent), outboundmessage.SentAtGTE(since)).Count(ctx))
	broadcasts := q.exists(client.Broadcast.Query().
		Where(broadcast.StatusEQ(broadcast.StatusSent), broadcast.SentAtGTE(since)).Exist(ctx))
	automations := q.exists(client.Automation.Query().Where(automation.StatusEQ(automation.StatusActive)).Exist(ctx))
	tracker := q.exists(client.Visitor.Query().Where(visitor.LastSeenAtGTE(since)).Exist(ctx))
	externalAPI := q.exists(client.ApiToken.Query().Where(apitoken.LastUsedAtGTE(since)).Exist(ctx))
	webhooks := q.exists(client.WebhookEndpoint.Query().Exist(ctx))
	providers, err := client.Integration.Query().Unique(true).Select(integration.FieldProvider).Strings(ctx)
	if err != nil {
		return telemetryapi.TelemetryReport{}, err
	}
	if q.err != nil {
		return telemetryapi.TelemetryReport{}, q.err
	}
	major, err := postgresMajor(ctx, client)
	if err != nil {
		return telemetryapi.TelemetryReport{}, err
	}
	kinds := make([]telemetryapi.TelemetryIntegrationKind, 0, len(providers))
	for _, p := range providers {
		switch integration.Provider(p) {
		case integration.ProviderSMTP:
			kinds = append(kinds, telemetryapi.TelemetryIntegrationKindSMTP)
		case integration.ProviderSes:
			kinds = append(kinds, telemetryapi.TelemetryIntegrationKindSes)
		}
	}
	slices.Sort(kinds)

	return telemetryapi.TelemetryReport{
		SchemaVersion:      schemaVersion,
		InstanceId:         row.InstanceID,
		AppVersion:         host.AppVersion,
		Edition:            host.Edition,
		Os:                 host.OS,
		Arch:               host.Arch,
		DeployShape:        host.DeployShape,
		PostgresMajor:      major,
		Locale:             host.Locale,
		InstanceAgeDays:    max(0, int32(now.Sub(row.CreatedAt)/(24*time.Hour))),
		Workspaces:         bucket(workspaces),
		Contacts:           bucket(contacts),
		EmailsSent30d:      bucket(emails),
		UsedBroadcasts30d:  broadcasts,
		AutomationsActive:  automations,
		UsedTracker30d:     tracker,
		UsedExternalApi30d: externalAPI,
		WebhooksConfigured: webhooks,
		IntegrationKinds:   kinds,
	}, nil
}

// reportQueries keeps the first error of a run of reads, so the builder reads top to bottom.
type reportQueries struct{ err error }

func (q *reportQueries) count(n int, err error) int {
	q.err = firstErr(q.err, err)
	return n
}

func (q *reportQueries) exists(ok bool, err error) bool {
	q.err = firstErr(q.err, err)
	return ok
}

func firstErr(first, next error) error {
	if first != nil {
		return first
	}
	return next
}

// bucket rounds a count to its order of magnitude: the one place an exact number is dropped.
func bucket(n int) telemetryapi.OrderOfMagnitude {
	switch {
	case n <= 0:
		return telemetryapi.OrderOfMagnitude0
	case n < 10:
		return telemetryapi.OrderOfMagnitude19
	case n < 100:
		return telemetryapi.OrderOfMagnitude1099
	case n < 1_000:
		return telemetryapi.OrderOfMagnitude100999
	case n < 10_000:
		return telemetryapi.OrderOfMagnitude10009999
	case n < 100_000:
		return telemetryapi.OrderOfMagnitude1000099999
	case n < 1_000_000:
		return telemetryapi.OrderOfMagnitude100000999999
	}
	return telemetryapi.OrderOfMagnitude1000000
}

// postgresMajor is the major version of the Postgres server (server_version_num is
// major*10000+minor from version 10 on).
func postgresMajor(ctx context.Context, client *ent.Client) (int32, error) {
	rows, err := client.QueryContext(ctx, "SHOW server_version_num")
	if err != nil {
		return 0, fmt.Errorf("read the Postgres version: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var num int32
	if !rows.Next() {
		return 0, fmt.Errorf("read the Postgres version: no row")
	}
	if err := rows.Scan(&num); err != nil {
		return 0, fmt.Errorf("read the Postgres version: %w", err)
	}
	return num / 10000, rows.Err()
}
