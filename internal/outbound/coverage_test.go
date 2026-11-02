package outbound_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/validation"
)

const staleClaimKey = "transactional:fixture-stale-claim"

// onOutboundMutation installs a hook on the OutboundMessage client the module under
// test shares, letting a test observe or fail a specific write the module makes.
func onOutboundMutation(env *testhelper.TestEnv, fn func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error)) {
	env.DB.OutboundMessage.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			om, ok := m.(*ent.OutboundMessageMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			return fn(ctx, om, next)
		})
	})
}

// captureLog routes slog.Default into a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// cancelingSenders resolves the sender but cancels the context on the way, so the
// next database call of the attempt fails.
type cancelingSenders struct {
	cancel func()
	sender messaging.EmailSender
}

func (s cancelingSenders) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
	s.cancel()
	return s.sender, nil
}

func TestDetailWordsEveryHold(t *testing.T) {
	assert.Equal(t, "sending is suspended for this workspace", outbound.Detail(holds.ReasonSuspended))
	assert.Equal(t, "no email integration can send: the one named is disabled or archived, or there is no default", outbound.Detail(outbound.HoldNoIntegration))
	assert.Equal(t, "sender domain is not a verified sending domain", outbound.Detail(outbound.HoldUnverifiedDomain))
	assert.Equal(t, "the email provider has stopped sending from this account", outbound.Detail(outbound.HoldProviderStopped))
	assert.Equal(t, "sending is currently on hold for this workspace: custom_reason", outbound.Detail("custom_reason"))
}

func TestSendRejectsIncompleteRequests(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	cases := map[string]struct {
		mutate func(*outbound.Request)
		want   string
	}{
		"no key":         {func(r *outbound.Request) { r.Key = "" }, "idempotency key required"},
		"no destination": {func(r *outbound.Request) { r.Destination = "   " }, "destination required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := transactional("tx:invalid", "a@example.com")
			tc.mutate(&req)
			_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
			assert.ErrorContains(t, err, tc.want)
		})
	}
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestSendInfrastructureErrorsAreReturnedNotDecided(t *testing.T) {
	t.Run("replay lookup fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:cancel", "a@example.com"))
		assert.Error(t, err)
	})

	t.Run("unknown workspace", func(t *testing.T) {
		env := testhelper.Setup(t)
		req := transactional("tx:nows", "a@example.com")
		_, err := newModule(env).Send(context.Background(), env.DB.Scoped(987654), req)
		assert.ErrorContains(t, err, "load workspace 987654")
	})

	t.Run("sender cannot be resolved", func(t *testing.T) {
		env := testhelper.Setup(t)
		m := outbound.New(env.Bus, env.Holds, senders{err: errors.New("vault down")}, nil)
		_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional("tx:nosender", "a@example.com"))
		require.ErrorContains(t, err, "resolve sender")
		assert.ErrorContains(t, err, "vault down")
	})

	t.Run("sending domain lookup fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := outbound.New(env.Bus, env.Holds, cancelingSenders{cancel: cancel, sender: env.CustomerMail}, nil)
		_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:domaindb", "a@example.com"))
		assert.ErrorContains(t, err, "check sending domain")
	})
}

// liftHold sets the expiry of the fixture Workspace's Staff hold, a real row: a future one
// makes it active, a past one lapses it.
func liftHold(t *testing.T, env *testhelper.TestEnv, expires time.Time) {
	t.Helper()
	env.DB.Hold.UpdateOneID(fixtures.HoldAcmeLapsedID).SetExpiresAt(expires).ExecX(context.Background())
}

// Send and Preflight read one gate and agree, on real rows, on an active Staff hold and a
// lapsed one; the send goes out again once the hold lapses.
func TestSendAndPreflightAgreeOnAStaffHold(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := newModule(env)
	agree := func(key, want string) {
		t.Helper()
		hold, err := m.Preflight(ctx, s, outbound.EmailContent{}, nil)
		require.NoError(t, err)
		res, err := m.Send(ctx, s, transactional(key, "a@example.com"))
		require.NoError(t, err)
		assert.Equal(t, want, hold, "Preflight")
		if want == "" {
			assert.Equal(t, outbound.Sent, res.Outcome, "Send")
			return
		}
		assert.Equal(t, outbound.Held, res.Outcome, "Send")
		assert.Equal(t, want, res.Reason, "Send")
		assert.Zero(t, res.MessageID, "a hold records nothing")
	}

	agree("tx:lapsed", "")
	liftHold(t, env, time.Now().Add(time.Hour))
	agree("tx:active", holds.ReasonStaff)
	assert.Len(t, env.CustomerMail.Messages(), 1, "only the lapsed-hold send went out")
	liftHold(t, env, time.Now().Add(-time.Hour))
	agree("tx:active", "")
}

func TestDeletionIsReportedBeforeSuspensionAndStaffHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := newModule(env)

	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
	res, err := m.Send(ctx, s, transactional("tx:susp", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, holds.ReasonSuspended, res.Reason)

	liftHold(t, env, time.Now().Add(time.Hour))
	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(ctx)
	res, err = m.Send(ctx, s, transactional("tx:both", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, holds.ReasonPendingDeletion, res.Reason)
	assert.Empty(t, env.CustomerMail.Messages())
}

// A core-only instance (no `staff` license) honors deletion and suspension and ignores Staff
// holds.
func TestACoreOnlyInstanceIgnoresStaffHoldsButHonorsDeletionAndSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := outbound.New(env.Bus, holds.New(func() bool { return false }, time.Now), senders{sender: env.CustomerMail},
		tracking.New("test-secret", "http://local"))
	liftHold(t, env, time.Now().Add(time.Hour))

	res, err := m.Send(ctx, s, transactional("tx:core", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome, "the Staff hold is ignored")

	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
	res, err = m.Send(ctx, s, transactional("tx:core-susp", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, holds.ReasonSuspended, res.Reason)

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(ctx)
	res, err = m.Send(ctx, s, transactional("tx:core-del", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, holds.ReasonPendingDeletion, res.Reason)
}

func TestWithLeaseControlsWhenAClaimCanBeTakenOver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	req := transactional(staleClaimKey, "lease.demo@codebasics.dev")

	// A claim ten minutes old is stale under the default five-minute lease but live
	// under an hour-long one.
	env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(staleClaimKey)).
		SetClaimedAt(time.Now().Add(-10 * time.Minute)).ExecX(ctx)
	_, err := newModule(env, outbound.WithLease(time.Hour)).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.Empty(t, env.CustomerMail.Messages())

	// MarkFailed honours the same lease: it leaves a claim someone may still be working on.
	require.NoError(t, newModule(env, outbound.WithLease(time.Hour)).MarkFailed(ctx, env.DB.Scoped(fixtures.AcmeID), staleClaimKey, errors.New("gave up")))
	assert.Equal(t, outboundmessage.StatusPending, byKey(t, env, staleClaimKey).Status)

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
}

func TestReplayReportsSkippedAndFailedOutcomes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	skipped := transactional("tx:skip", suppressd)
	first, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), skipped)
	require.NoError(t, err)
	require.Equal(t, outbound.Skipped, first.Outcome)
	assert.Equal(t, eligibility.ReasonSuppressed, first.Reason)
	again, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), skipped)
	require.NoError(t, err)
	assert.Equal(t, outbound.Skipped, again.Outcome)
	assert.Equal(t, eligibility.ReasonSuppressed, again.Reason)
	assert.True(t, again.Replayed)
	assert.Equal(t, first.MessageID, again.MessageID)

	broken := transactional("tx:broken", "a@example.com")
	broken.Content = outbound.EmailContent{Subject: "{% if %}broken", Body: htmlBody}
	first, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), broken)
	require.NoError(t, err)
	require.Equal(t, outbound.Failed, first.Outcome)
	again, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), broken)
	require.NoError(t, err)
	assert.Equal(t, outbound.Failed, again.Outcome)
	assert.Equal(t, first.Reason, again.Reason)
	assert.True(t, again.Replayed)
}

func TestComposeBindsContactFieldsWithRequestVariablesWinning(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	first, last := "Ada", "Lovelace"
	email := "ada@example.com"

	req := transactional("tx:bind", email)
	req = withSubject(req, "{{ nick }}|{{ first_name }}|{{ last_name }}|{{ email }}|{{ plan }}")
	req.Contact = &ent.Contact{
		Email: &email, FirstName: &first, LastName: &last,
		// A custom field must never shadow a core field; the Request wins over both.
		CustomFields: map[string]any{"nick": "countess", "first_name": "SHADOW", "plan": "free"},
	}
	req.Variables = map[string]any{"plan": "pro"}
	res, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "countess|Ada|Lovelace|ada@example.com|pro", msgs[0].Subject)

	// A Contact with no core fields binds them as empty, not as a missing variable.
	bare := transactional("tx:bind-bare", "bare@example.com")
	bare = withSubject(bare, "[{{ first_name }}][{{ email }}]")
	bare.Contact = &ent.Contact{}
	_, err = m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), bare)
	require.NoError(t, err)
	msgs = env.CustomerMail.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "[][]", msgs[1].Subject)
}

func TestTrackedMarketingSendRewritesLinksAndAddsPixel(t *testing.T) {
	env := testhelper.Setup(t)
	req := marketing(t, env, "bc:tracked", fixtures.ContactAliceID)
	req.Track = true
	req = withBody(req, `<p><a href="https://example.com/x">go</a></p>`)

	res, err := newModule(env).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, "/e/c/", "links go through the click tracker")
	assert.Contains(t, msgs[0].HTML, "/e/o/", "open pixel is embedded")
	assert.Contains(t, msgs[0].HTML, "unsubscribe")
	assert.NotEmpty(t, msgs[0].ListUnsubscribeURL)
}

func TestClaimRecordsEveryRefOnTheMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	step := 3
	req := transactional("tx:refs", "refs@example.com")
	req.ContactID = fixtures.ContactAliceID
	req.Ref = outbound.Ref{
		BroadcastID: fixtures.BroadcastSentID, BroadcastRecipient: fixtures.BroadcastRecipientSentID, AutomationID: fixtures.AutomationWelcomeSeriesID,
		AutomationRunID: fixtures.AutomationRunWelcomeCompletedID, AutomationStep: &step, TemplateID: fixtures.TemplateWelcomeID,
	}

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)

	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.Equal(t, int64(fixtures.BroadcastSentID), *row.BroadcastID)
	assert.Equal(t, int64(fixtures.BroadcastRecipientSentID), *row.BroadcastRecipientID)
	assert.Equal(t, int64(fixtures.AutomationWelcomeSeriesID), *row.AutomationID)
	assert.Equal(t, int64(fixtures.AutomationRunWelcomeCompletedID), *row.AutomationRunID)
	assert.Equal(t, 3, *row.AutomationStep)
	assert.Equal(t, int64(fixtures.TemplateWelcomeID), *row.TemplateID)
	assert.Equal(t, int64(fixtures.ContactAliceID), *row.ContactID)
}

func TestClaimErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("insert fails for a reason other than the key", func(t *testing.T) {
		env := testhelper.Setup(t)
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) {
				return nil, errors.New("disk full")
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:insert", "a@example.com"))
		require.ErrorContains(t, err, "disk full")
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("lost the insert race to a finished send", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !raced {
				raced = true
				// Another attempt records the same key as sent between our replay check and insert.
				// test writes: a concurrent write simulated inside a hook is the race under test
				env.DB.OutboundMessage.Create().SetWorkspaceID(fixtures.AcmeID).SetKind(outboundmessage.KindTransactional).
					SetIdempotencyKey("tx:race").SetDestination("a@example.com").SetClaimedAt(time.Now()).
					SetStatus(outboundmessage.StatusSent).ExecX(ctx)
				// The real insert would now violate the unique key; Postgres would also abort the
				// whole test transaction, so report the violation the way ent does instead.
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
		require.NoError(t, err)
		assert.Equal(t, outbound.Sent, res.Outcome)
		assert.True(t, res.Replayed)
		assert.Empty(t, env.CustomerMail.Messages(), "the winner already sent it")
	})

	t.Run("lost the insert race to a live claim", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !raced {
				raced = true
				// test writes: a concurrent write simulated inside a hook is the race under test
				env.DB.OutboundMessage.Create().SetWorkspaceID(fixtures.AcmeID).SetKind(outboundmessage.KindTransactional).
					SetIdempotencyKey("tx:race").SetDestination("a@example.com").SetClaimedAt(time.Now()).ExecX(ctx)
				// The real insert would now violate the unique key; Postgres would also abort the
				// whole test transaction, so report the violation the way ent does instead.
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
		require.ErrorIs(t, err, outbound.ErrInProgress)
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("lost the insert race to an abandoned claim and adopts it", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !raced {
				raced = true
				// test writes: a concurrent write simulated inside a hook is the race under test
				env.DB.OutboundMessage.Create().SetWorkspaceID(fixtures.AcmeID).SetKind(outboundmessage.KindTransactional).
					SetIdempotencyKey("tx:race").SetDestination("a@example.com").
					SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
				// The real insert would now violate the unique key; Postgres would also abort the
				// whole test transaction, so report the violation the way ent does instead.
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
		require.NoError(t, err)
		assert.Equal(t, outbound.Sent, res.Outcome)
		assert.Len(t, env.CustomerMail.Messages(), 1)
	})

	t.Run("constraint error but no row under the key", func(t *testing.T) {
		env := testhelper.Setup(t)
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) {
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:ghost", "a@example.com"))
		assert.True(t, ent.IsConstraintError(err), "the original insert error is surfaced")
	})

	t.Run("re-reading the winner fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		failFind := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) {
				failFind = true
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		env.DB.OutboundMessage.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
				if failFind {
					return nil, errors.New("replica lag")
				}
				return next.Query(ctx, q)
			})
		}))
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:findfail", "a@example.com"))
		assert.ErrorContains(t, err, "replica lag")
	})

	t.Run("adopting a stale claim fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdate) {
				return nil, errors.New("write refused")
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional(staleClaimKey, "lease.demo@codebasics.dev"))
		require.ErrorContains(t, err, "write refused")
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("a competing retry adopts the stale claim first", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdate) && !raced {
				raced = true
				// The other retry refreshes the claim token just before our conditional update.
				env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(staleClaimKey)).
					SetClaimedAt(time.Now().Add(-30 * time.Minute)).ExecX(ctx)
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional(staleClaimKey, "lease.demo@codebasics.dev"))
		require.ErrorIs(t, err, outbound.ErrInProgress)
		assert.Empty(t, env.CustomerMail.Messages())
	})
}

func TestEligibilityFailureReleasesTheClaimAndFailsClosed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	logs := captureLog(t)

	// The eligibility rule runs through the Workspace's Suppression query.
	env.DB.Suppression.Intercept(ent.InterceptFunc(func(_ ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(_ context.Context, _ ent.Query) (ent.Value, error) {
			return nil, errors.New("rule store unavailable")
		})
	}))
	// And the release of the claim itself fails, which is only logged.
	onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
		if m.Op().Is(ent.OpUpdate) {
			return nil, errors.New("release refused")
		}
		return next.Mutate(ctx, m)
	})

	_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:elig", "a@example.com"))
	require.ErrorContains(t, err, "rule store unavailable")
	assert.Empty(t, env.CustomerMail.Messages(), "fail closed: nothing is sent when eligibility cannot be checked")
	assert.Contains(t, logs.String(), "outbound: release claim failed")
	assert.Contains(t, logs.String(), "release refused")
}

func TestFinishErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("recording a skip fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if st, ok := m.Status(); ok && st == outboundmessage.StatusSkipped {
				return nil, errors.New("write refused")
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:finish", suppressd))
		assert.ErrorContains(t, err, "write refused")
	})

	t.Run("the claim was taken over while we decided", func(t *testing.T) {
		env := testhelper.Setup(t)
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if st, ok := m.Status(); ok && st == outboundmessage.StatusSkipped {
				env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey("tx:finish")).
					SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:finish", suppressd))
		assert.ErrorIs(t, err, outbound.ErrInProgress)
	})
}

// A stale claim token cannot write any final status: whichever attempt took the claim over
// owns the row, and the late one is told so (ErrInProgress) and leaves it untouched.
func TestAStaleClaimTokenCannotWriteFailed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	req := transactional("tx:stale-failed", "a@example.com")
	req = withSubject(req, "{% if %}broken")
	onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
		if st, ok := m.Status(); ok && st == outboundmessage.StatusFailed {
			// Another attempt takes the claim over just before this one writes.
			env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(req.Key)).
				SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
		}
		return next.Mutate(ctx, m)
	})

	_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)

	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.Equal(t, outboundmessage.StatusPending, byKey(t, env, req.Key).Status)
}

// takeoverSender is a provider whose call outlives the lease: another attempt takes the claim
// over while it runs.
type takeoverSender struct {
	messaging.EmailSender

	takeOver func(context.Context)
}

func (s takeoverSender) DefaultFrom() (address, name string) {
	return s.EmailSender.(messaging.DefaultFromer).DefaultFrom()
}

func (s takeoverSender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	s.takeOver(ctx)
	return s.EmailSender.Send(ctx, msg)
}

func TestAStaleClaimTokenCannotWriteSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	req := transactional("tx:stale-sent", "a@example.com")
	slow := takeoverSender{EmailSender: env.CustomerMail, takeOver: func(ctx context.Context) {
		env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(req.Key)).
			SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
	}}
	m := outbound.New(env.Bus, env.Holds, senders{sender: slow}, tracking.New("test-secret", "http://local"))

	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)

	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.Equal(t, outboundmessage.StatusPending, byKey(t, env, req.Key).Status)
}

func TestPreflightErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := newModule(env).Preflight(ctx, env.DB.Scoped(987654), outbound.EmailContent{}, nil)
	require.ErrorContains(t, err, "freeze check")

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = newModule(env).Preflight(canceled, env.DB.Scoped(fixtures.AcmeID), outbound.EmailContent{}, nil)
	assert.ErrorContains(t, err, "freeze check")
}

func TestSendTestOutcomes(t *testing.T) {
	ctx := context.Background()
	req := outbound.TestRequest{To: "Preview@Example.com",
		Content: outbound.EmailContent{Subject: "s", Body: htmlBody}}

	t.Run("unknown workspace", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := newModule(env).SendTest(ctx, env.DB.Scoped(987654), req)
		assert.ErrorContains(t, err, "load workspace 987654")
	})

	t.Run("content that does not render is Failed", func(t *testing.T) {
		env := testhelper.Setup(t)
		bad := req
		bad.Content.Subject = "{% if %}broken"
		res, err := newModule(env).SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), bad)
		require.NoError(t, err)
		assert.Equal(t, outbound.Failed, res.Outcome)
		assert.NotEmpty(t, res.Reason)
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("destination is normalized", func(t *testing.T) {
		env := testhelper.Setup(t)
		res, err := newModule(env).SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		require.NoError(t, err)
		assert.Equal(t, outbound.Sent, res.Outcome)
		require.Len(t, env.CustomerMail.Messages(), 1)
		assert.Equal(t, "preview@example.com", env.CustomerMail.Messages()[0].To)
	})

	t.Run("domain loses verification at sign time", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.CustomerMail.SetErr(messaging.ErrUnverifiedSendingDomain)
		res, err := newModule(env).SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		require.NoError(t, err)
		assert.Equal(t, outbound.Held, res.Outcome)
		assert.Equal(t, outbound.HoldUnverifiedDomain, res.Reason)
	})

	t.Run("provider error is returned", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.CustomerMail.SetErr(errors.New("smtp unavailable"))
		_, err := newModule(env).SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		require.ErrorContains(t, err, "test send to preview@example.com")
		assert.ErrorContains(t, err, "smtp unavailable")
	})
}

func TestSendBroadcastTestRefusesEveryOutcomeButSent(t *testing.T) {
	ctx := context.Background()
	from, fromName := "hello@codebasics.dev", "CodeBasics"
	broadcast := func(subject, body string) *ent.Broadcast {
		return &ent.Broadcast{WorkspaceID: fixtures.AcmeID, Subject: subject, Body: body, FromEmail: &from, FromName: &fromName}
	}

	t.Run("sent", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("Launch {{ first_name }}", htmlBody), "me@example.com", nil)
		require.NoError(t, err)

		msgs := env.CustomerMail.Messages()
		require.Len(t, msgs, 1)
		assert.Equal(t, "[Test] Launch Alex", msgs[0].Subject, "marked as a test, merged with sample data")
		assert.Equal(t, "me@example.com", msgs[0].To)
		assert.Contains(t, msgs[0].HTML, "Hi Alex")
		assert.Equal(t, "hello@codebasics.dev", msgs[0].From)
	})

	t.Run("held", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
		_, err := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("s", htmlBody), "me@example.com", nil)
		require.ErrorIs(t, err, outbound.ErrHeld)
		assert.Equal(t, "the message was not sent: "+outbound.Detail(holds.ReasonSuspended), validation.Message(err))
	})

	t.Run("send error", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.CustomerMail.SetErr(errors.New("smtp unavailable"))
		_, err := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("s", htmlBody), "me@example.com", nil)
		require.ErrorContains(t, err, "smtp unavailable")
		assert.NotErrorIs(t, err, outbound.ErrHeld, "a failed send is a fault, not a refusal")
	})

	t.Run("content does not render", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("{% if %}broken", htmlBody), "me@example.com", nil)
		require.ErrorIs(t, err, emailrender.ErrInvalidContent)
		assert.Empty(t, env.CustomerMail.Messages())
	})
}
