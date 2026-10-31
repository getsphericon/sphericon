package abuse

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// Interval is how often the detector runs.
const Interval = time.Hour

// Config is what a Detector is built from.
type Config struct {
	Limits Limits
	// SupportEmail is the contact the notices give an owner to appeal to; empty gives none.
	SupportEmail string
}

// Detector performs the detector's pass (ADR 0026). It is the raw-client entry of the
// control: like a job entry point it lists Workspaces, then scopes to each one.
type Detector struct {
	*Watch

	client  *ent.Client
	bus     *events.Bus
	sender  messaging.EmailSender
	support string
}

// New builds the Detector. The sender is the platform's (system) sender: the notices
// never go through a Workspace's own Integration, which a freeze holds.
func New(client *ent.Client, lic *licensekey.License, bus *events.Bus, sender messaging.EmailSender, cfg Config) *Detector {
	return &Detector{Watch: NewWatch(lic, cfg.Limits), client: client, bus: bus, sender: sender, support: cfg.SupportEmail}
}

// Summary is who a pass warned and who it froze.
type Summary struct {
	Warned, Frozen []int64
}

// Run is one pass at now over every Workspace. Unlicensed it does nothing. Per Workspace,
// in this order: a suspended one is skipped (its first actor and reason stand), an exempt
// one is skipped, then the Workspace is judged over the window since its last clearance.
// A freeze suspends it as `system`; a warning mails the owners once per episode, and
// leaving the warning tier ends the episode. It never lifts a suspension. A failed notice
// never undoes a freeze: it is logged, and a warning whose mail failed is retried next
// pass. One Workspace's failure does not stop the others; the errors are joined.
func (d *Detector) Run(ctx context.Context, now time.Time) (Summary, error) {
	var sum Summary
	if !d.Licensed() {
		return sum, nil
	}
	// A Workspace that was warned and then suspended by hand or exempted has left the
	// detector's reach: end its episode, so a stale marker neither lists it as warned nor
	// mutes the warning it earns after being cleared.
	var errs []error
	if err := d.client.Workspace.Update().
		Where(workspace.AbuseWarningNotifiedAtNotNil(), workspace.Or(suspension.Suspended(), workspace.AbuseExempt(true))).
		ClearAbuseWarningNotifiedAt().
		Exec(ctx); err != nil {
		errs = append(errs, fmt.Errorf("end the warning episodes of suspended and exempt workspaces: %w", err))
	}
	candidates, err := d.client.Workspace.Query().
		Where(suspension.NotSuspended(), workspace.AbuseExempt(false), accounts.Live()).
		All(ctx)
	if err != nil {
		return sum, errors.Join(append(errs, fmt.Errorf("list workspaces for the abuse detector: %w", err))...)
	}
	for _, ws := range candidates {
		breach, err := d.Standing(ctx, d.client.Scoped(ws.ID), ws, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("rates of workspace %d: %w", ws.ID, err))
			continue
		}
		switch breach.Tier {
		case TierFreeze:
			frozen, err := d.freeze(ctx, ws, breach)
			if frozen {
				sum.Frozen = append(sum.Frozen, ws.ID)
			}
			if err != nil {
				errs = append(errs, err)
			}
		case TierWarn:
			warned, err := d.warn(ctx, ws, breach, now)
			if warned {
				sum.Warned = append(sum.Warned, ws.ID)
			}
			if err != nil {
				errs = append(errs, err)
			}
		default:
			if ws.AbuseWarningNotifiedAt != nil {
				if err := d.client.Workspace.UpdateOneID(ws.ID).ClearAbuseWarningNotifiedAt().Exec(ctx); err != nil {
					errs = append(errs, fmt.Errorf("end the warning episode of workspace %d: %w", ws.ID, err))
				}
			}
		}
	}
	return sum, errors.Join(errs...)
}

// freeze suspends the Workspace as the system and tells its owners. Freezing is itself
// the escalation of a warning, so no warning mail goes out and the episode ends. A failed
// notice is logged, not returned: the freeze stands and nothing retries a suspended
// Workspace.
func (d *Detector) freeze(ctx context.Context, ws *ent.Workspace, b Breach) (bool, error) {
	changed, err := suspension.SuspendWorkspace(ctx, d.bus, ws.ID, suspension.System, b.Describe()+" (freeze)")
	if err != nil || !changed {
		return false, err
	}
	if err := d.client.Workspace.UpdateOneID(ws.ID).ClearAbuseWarningNotifiedAt().Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "workspace frozen, but its warning episode could not be ended", "workspace_id", ws.ID, "error", err)
	}
	if err := suspension.NotifyOwners(ctx, d.client, accounts.OwnerNotifier(d.client, d.sender), d.support, ws.ID); err != nil {
		slog.ErrorContext(ctx, "workspace frozen by the abuse detector, but the owner notice could not be sent", "workspace_id", ws.ID, "error", err)
	}
	return true, nil
}

// metricNames words a breached rate in the warning to the owners.
var metricNames = map[Metric]i18n.ID{Complaint: i18n.MetricComplaint, Bounce: i18n.MetricBounce}

// warn mails the owners once per episode: the marker is set only after the mail went, so
// a failed send is retried, and is cleared when the Workspace leaves the tier.
func (d *Detector) warn(ctx context.Context, ws *ent.Workspace, b Breach, now time.Time) (bool, error) {
	if ws.AbuseWarningNotifiedAt != nil || d.sender == nil {
		return false, nil
	}
	data := map[string]any{
		"Workspace": ws.Name, "Domain": b.Domain, "Metric": i18n.T(metricNames[b.Metric], nil),
		"Rate": Percent(b.Rate()), "Count": b.Numerator, "Total": b.Denominator,
		"Limit": Percent(float64(b.LimitBP) / 10000), "Support": d.support,
	}
	if err := accounts.OwnerNotifier(d.client, d.sender).Tell(ctx, ws.ID, ownernotice.Notice{Subject: i18n.EmailAbuseWarningSubject, Body: i18n.EmailAbuseWarningBody}, data); err != nil {
		return false, fmt.Errorf("warn workspace %d: %w", ws.ID, err)
	}
	if err := d.client.Workspace.UpdateOneID(ws.ID).SetAbuseWarningNotifiedAt(now).Exec(ctx); err != nil {
		return false, fmt.Errorf("record the warning of workspace %d: %w", ws.ID, err)
	}
	return true, nil
}

// PassArgs is the hourly detector job.
type PassArgs struct{}

func (PassArgs) Kind() string { return "ee_abuse_detector" }

// Worker runs Run on the periodic tick.
type Worker struct {
	river.WorkerDefaults[PassArgs]

	detector *Detector
}

// NewWorker builds the detector worker.
func NewWorker(d *Detector) *Worker { return &Worker{detector: d} }

func (w *Worker) Work(ctx context.Context, _ *river.Job[PassArgs]) error {
	_, err := w.detector.Run(ctx, time.Now())
	return err
}

// Periodic is the river schedule of the detector.
func Periodic() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(Interval),
		func() (river.JobArgs, *river.InsertOpts) { return PassArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}
