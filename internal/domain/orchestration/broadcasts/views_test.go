package broadcasts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The views answer with what the row alone does not hold: the figures of every Broadcast of a
// page, and of the one read.
func TestViewsCarryTheFigures(t *testing.T) {
	env := testhelper.Setup(t)
	v := broadcasts.NewViews(broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()))
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	page, err := v.List(ctx, s, pagination.Params{Page: 1, PageSize: 100}, broadcasts.ListFilter{}, broadcasts.OrderNewest)
	require.NoError(t, err)
	sent, ok := lo.Find(page.Items, func(b broadcasts.View) bool { return b.ID == fixtures.BroadcastSentID })
	require.True(t, ok)
	assert.Equal(t, 11, sent.Stats.Sent)
	assert.Nil(t, sent.Progress, "a list leaves the Progress out")

	got, err := v.Get(ctx, s, fixtures.BroadcastSentID)
	require.NoError(t, err)
	assert.Equal(t, 11, got.Stats.Sent)
	assert.Nil(t, got.Progress, "a sent Broadcast has no Progress")
}

// A save through the views returns the lint of the email body as written, against the Workspace's
// Brand kit (ADR 0040).
func TestViewsCreateReturnsTheWarningsOfTheBodyAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	v := broadcasts.NewViews(broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()))
	name, subject, from := "Warned", "Hi", "a@"+fixtures.SendingDomainVerifiedDomain
	body := `<p style="font-family:Georgia, serif">Sale</p><marquee>Scroll</marquee>`

	got, err := v.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		broadcasts.CreateInput{Name: &name, Content: broadcasts.EmailContentInput{Subject: &subject, Body: &body, FromEmail: &from}})
	require.NoError(t, err)
	codes := lo.Map(got.Warnings, func(f emailrender.Finding, _ int) emailrender.Code { return f.Code })
	assert.ElementsMatch(t, []emailrender.Code{emailrender.CodeOffBrandFont, emailrender.CodeUnknownConstruct}, codes)
	assert.NotContains(t, *got.Body, "marquee", "stored sanitized")
}
