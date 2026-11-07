package jobs_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// sesReply is what the stand-in SES API answers a send with: the HTTP status and the
// x-amzn-ErrorType header the SES v2 REST protocol names an error by.
type sesReply struct {
	status             int
	errorType, message string
}

// sesReplying is the real SES provider pointed at a stand-in API that answers every send with reply.
func sesReplying(t *testing.T, reply sesReply) messaging.EmailSender {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Amzn-Errortype", reply.errorType)
		w.WriteHeader(reply.status)
		_, _ = fmt.Fprintf(w, `{"message":%q}`, reply.message)
	}))
	t.Cleanup(srv.Close)

	raw := fmt.Sprintf(`{"region":"us-east-2","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","endpoint":%q}`, srv.URL)
	built, err := ses.NewDescriptor(http.DefaultClient).Build([]byte(raw), messaging.Deps{})
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	return sender
}

// failedNotice sends a Notice through river's real runtime, the System sender failing with what
// the real SES provider makes of reply, and returns the job once its first attempt has failed:
// what river stored for it.
func failedNotice(t *testing.T, reply sesReply) *rivertype.JobRow {
	t.Helper()
	ctx := context.Background()
	_, sendErr := sesReplying(t, reply).Send(ctx, messaging.EmailMessage{To: "a@example.com", Subject: "s", Text: "x"})
	require.Error(t, sendErr)

	e := newRiverEnvWithRealQueue(t)
	e.SystemMail.SetErr(sendErr)
	require.NoError(t, e.client.Start(ctx))
	t.Cleanup(func() { cleanup.Report("stop the job client", e.client.Stop(context.Background())) })
	e.insert(t, jobs.SendNoticeArgs{EventID: "01NOTICE", To: "a@example.com", Subject: "Reset your password", Text: "link"})

	var row *rivertype.JobRow
	require.Eventually(t, func() bool {
		res, err := e.queue.JobList(ctx, river.NewJobListParams().Kinds(jobs.SendNoticeArgs{}.Kind()))
		if err != nil || len(res.Jobs) == 0 {
			return false
		}
		row = res.Jobs[0]
		return len(row.Errors) > 0
	}, 10*time.Second, 50*time.Millisecond, "the first attempt fails")
	return row
}

// A Notice the provider refuses for a reason no retry changes (the sender's IAM user may not send
// as the identity, the identity is not verified) is cancelled, not retried 25 times over a day and
// then dropped: a retry of a refusal is the same refusal.
func TestNoticeCancelledWhenNoRetryCuresTheRefusal(t *testing.T) {
	for name, reply := range map[string]sesReply{
		"the sender may not send as the identity": {http.StatusForbidden, "AccessDeniedException", "User is not authorized to perform 'ses:SendRawEmail' on resource identity/bill@hexlet.io"},
		"the message is rejected":                 {http.StatusBadRequest, "MessageRejected", "Email address is not verified."},
		"the sending domain is not verified":      {http.StatusBadRequest, "MailFromDomainNotVerifiedException", "Mail-from domain not verified."},
	} {
		t.Run(name, func(t *testing.T) {
			row := failedNotice(t, reply)
			assert.Equal(t, rivertype.JobStateCancelled, row.State)
		})
	}
}

// What a retry does cure keeps its retries: a throttled send, a quota that refills, a provider
// that is down.
func TestNoticeRetriedWhenARetryCuresTheFailure(t *testing.T) {
	for name, reply := range map[string]sesReply{
		"throttled":            {http.StatusTooManyRequests, "TooManyRequestsException", "Maximum sending rate exceeded."},
		"daily quota spent":    {http.StatusTooManyRequests, "TooManyRequestsException", "Daily message quota exceeded."},
		"the provider is down": {http.StatusInternalServerError, "InternalFailure", "try again"},
	} {
		t.Run(name, func(t *testing.T) {
			row := failedNotice(t, reply)
			assert.NotContains(t, []rivertype.JobState{rivertype.JobStateCancelled, rivertype.JobStateDiscarded}, row.State,
				"a transient failure stays queued for its next attempt")
		})
	}
}
