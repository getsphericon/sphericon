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
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
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

type freezer struct {
	reason string
	err    error
	calls  int
}

func (f *freezer) Frozen(context.Context, *ent.Scoped, *ent.Workspace) (string, error) {
	f.calls++
	return f.reason, f.err
}

// cancelingSenders resolves the sender but cancels the context on the way, so the
// next database call of the attempt fails.
type cancelingSenders struct {
	cancel func()
	sender messaging.EmailSender
}

func (s cancelingSenders) EmailSender(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
	s.cancel()
	return s.sender, nil
}

func TestHoldDetailWordsEveryHold(t *testing.T) {
	assert.Equal(t, "sending is suspended for this workspace", outbound.HoldDetail(outbound.HoldSuspended))
	assert.Equal(t, "no default email provider configured", outbound.HoldDetail(outbound.HoldNoIntegration))
	assert.Equal(t, "sender domain is not a verified sending domain", outbound.HoldDetail(outbound.HoldUnverifiedDomain))
	assert.Equal(t, "sending is currently on hold for this workspace: custom_reason", outbound.HoldDetail("custom_reason"))
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
		m := outbound.New(env.Bus, senders{err: errors.New("vault down")}, nil)
		_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional("tx:nosender", "a@example.com"))
		assert.ErrorContains(t, err, "resolve sender")
		assert.ErrorContains(t, err, "vault down")
	})

	t.Run("sending domain lookup fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := outbound.New(env.Bus, cancelingSenders{cancel: cancel, sender: env.CustomerMail}, nil)
		_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:domaindb", "a@example.com"))
		assert.ErrorContains(t, err, "check sending domain")
	})

	t.Run("freeze check fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		m := newModule(env, outbound.WithFreezers(&freezer{err: errors.New("hold source down")}))
		_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional("tx:freezeerr", "a@example.com"))
		assert.ErrorContains(t, err, "freeze check")
		assert.Empty(t, env.CustomerMail.Messages())
	})
}

func TestFreezersHoldAfterTheCoreSuspensionCheck(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	open := &freezer{}
	held := &freezer{reason: "operator_hold"}
	m := newModule(env, outbound.WithFreezers(open), outbound.WithFreezers(held))

	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:frozen", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	assert.Equal(t, "operator_hold", res.Reason)
	assert.Zero(t, res.MessageID, "a freeze hold records nothing")
	assert.Empty(t, env.CustomerMail.Messages())
	assert.Equal(t, 1, open.calls, "freezers run in registration order, across WithFreezers calls")

	hold, err := m.Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), "")
	require.NoError(t, err)
	assert.Equal(t, "operator_hold", hold, "Preflight applies the same gate")

	held.reason = ""
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:frozen", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome, "the same Request goes out once the freeze lifts")
}

// Send and Preflight read one gate: with the real holds.Freezer they agree on an
// active hold, a lapsed one and a hold the license does not honour.
func TestSendAndPreflightAgreeOnAnOperatorHold(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	honoured := true
	m := newModule(env, outbound.WithFreezers(holds.Freezer{Enabled: func() bool { return honoured }}))
	create := func(key string, expires time.Time) {
		_, err := s.Hold().Create().SetKey(key).SetReasonCode("review").SetMessage("held").
			SetSetBy("service-a").SetExpiresAt(expires).Save(ctx)
		require.NoError(t, err)
	}
	agree := func(key, want string) {
		t.Helper()
		hold, err := m.Preflight(ctx, s, "")
		require.NoError(t, err)
		res, err := m.Send(ctx, s, transactional(key, "a@example.com"))
		require.NoError(t, err)
		assert.Equal(t, want, hold, "Preflight")
		if want == "" {
			assert.Equal(t, outbound.Sent, res.Outcome, "Send")
		} else {
			assert.Equal(t, outbound.Held, res.Outcome, "Send")
			assert.Equal(t, want, res.Reason, "Send")
		}
	}

	// The fixture Workspace already carries a lapsed hold.
	agree("tx:lapsed", "")
	create("active", time.Now().Add(time.Hour))
	agree("tx:active", outbound.HoldOperator)
	honoured = false
	agree("tx:unlicensed", "")
}

func TestSuspensionIsCheckedBeforeFreezers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	f := &freezer{}
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).ExecX(ctx)

	res, err := newModule(env, outbound.WithFreezers(f)).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:susp", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.HoldSuspended, res.Reason)
	assert.Zero(t, f.calls)
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
	broken.Subject = "{% if %}broken"
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
	req.Subject = "{{ nick }}|{{ first_name }}|{{ last_name }}|{{ email }}|{{ plan }}"
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
	bare.Subject = "[{{ first_name }}][{{ email }}]"
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
	req.TrackID = 4242
	req.Body = `<mjml><mj-body><mj-section><mj-column><mj-text><a href="https://example.com/x">go</a></mj-text></mj-column></mj-section></mj-body></mjml>`

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
		assert.ErrorContains(t, err, "disk full")
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("lost the insert race to a finished send", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !raced {
				raced = true
				// Another attempt records the same key as sent between our replay check and insert.
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
				env.DB.OutboundMessage.Create().SetWorkspaceID(fixtures.AcmeID).SetKind(outboundmessage.KindTransactional).
					SetIdempotencyKey("tx:race").SetDestination("a@example.com").SetClaimedAt(time.Now()).ExecX(ctx)
				// The real insert would now violate the unique key; Postgres would also abort the
				// whole test transaction, so report the violation the way ent does instead.
				return nil, &ent.ConstraintError{}
			}
			return next.Mutate(ctx, m)
		})
		_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
		assert.ErrorIs(t, err, outbound.ErrInProgress)
		assert.Empty(t, env.CustomerMail.Messages())
	})

	t.Run("lost the insert race to an abandoned claim and adopts it", func(t *testing.T) {
		env := testhelper.Setup(t)
		raced := false
		onOutboundMutation(env, func(ctx context.Context, m *ent.OutboundMessageMutation, next ent.Mutator) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !raced {
				raced = true
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
		assert.ErrorContains(t, err, "write refused")
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
		assert.ErrorIs(t, err, outbound.ErrInProgress)
		assert.Empty(t, env.CustomerMail.Messages())
	})
}

func TestEligibilityFailureReleasesTheClaimAndFailsClosed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	logs := captureLog(t)

	// The eligibility rule runs through the Workspace's Suppression query.
	env.DB.Suppression.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
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

func TestPreflightErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := newModule(env).Preflight(ctx, env.DB.Scoped(987654), "")
	assert.ErrorContains(t, err, "load workspace 987654")

	_, err = newModule(env, outbound.WithFreezers(&freezer{err: errors.New("down")})).Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), "")
	assert.ErrorContains(t, err, "freeze check")
}

func TestSendTestOutcomes(t *testing.T) {
	ctx := context.Background()
	req := outbound.TestRequest{To: "Preview@Example.com", Subject: "s", Body: mjml}

	t.Run("unknown workspace", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := newModule(env).SendTest(ctx, env.DB.Scoped(987654), req)
		assert.ErrorContains(t, err, "load workspace 987654")
	})

	t.Run("gate error", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := newModule(env, outbound.WithFreezers(&freezer{err: errors.New("down")})).SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		assert.ErrorContains(t, err, "freeze check")
	})

	t.Run("content that does not render is Failed", func(t *testing.T) {
		env := testhelper.Setup(t)
		bad := req
		bad.Subject = "{% if %}broken"
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
		assert.ErrorContains(t, err, "test send to preview@example.com")
		assert.ErrorContains(t, err, "smtp unavailable")
	})
}

func TestSendBroadcastTestWordsEveryOutcome(t *testing.T) {
	ctx := context.Background()
	from, fromName := "hello@codebasics.dev", "CodeBasics"
	broadcast := func(subject, body string) *ent.Broadcast {
		return &ent.Broadcast{WorkspaceID: fixtures.AcmeID, Subject: subject, Body: body, FromEmail: &from, FromName: &fromName}
	}

	t.Run("sent", func(t *testing.T) {
		env := testhelper.Setup(t)
		got := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("Launch {{ first_name }}", mjml), "me@example.com")
		assert.Empty(t, got)

		msgs := env.CustomerMail.Messages()
		require.Len(t, msgs, 1)
		assert.Equal(t, "[Test] Launch Alex", msgs[0].Subject, "marked as a test, merged with sample data")
		assert.Equal(t, "me@example.com", msgs[0].To)
		assert.Contains(t, msgs[0].HTML, "Hi Alex")
		assert.Equal(t, "hello@codebasics.dev", msgs[0].From)
	})

	t.Run("held", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).ExecX(ctx)
		got := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("s", mjml), "me@example.com")
		assert.Equal(t, outbound.HoldDetail(outbound.HoldSuspended), got)
	})

	t.Run("send error", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.CustomerMail.SetErr(errors.New("smtp unavailable"))
		got := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("s", mjml), "me@example.com")
		assert.Contains(t, got, "send failed:")
		assert.Contains(t, got, "smtp unavailable")
	})

	t.Run("content does not render", func(t *testing.T) {
		env := testhelper.Setup(t)
		got := newModule(env).SendBroadcastTest(ctx, env.DB.Scoped(fixtures.AcmeID), broadcast("{% if %}broken", mjml), "me@example.com")
		assert.NotEmpty(t, got)
		assert.NotContains(t, got, "send failed")
		assert.Empty(t, env.CustomerMail.Messages())
	})
}
