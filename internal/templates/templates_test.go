package templates_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListIsAscendingByIDAndPaged(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	p, err := m.List(ctx, s, pagination.Params{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, p.Items, 2)
	assert.EqualValues(t, fixtures.TemplateWelcomeID, p.Items[0].ID)
	assert.Less(t, p.Items[0].ID, p.Items[1].ID)
	assert.GreaterOrEqual(t, p.TotalItems, 3)
	for _, it := range p.Items {
		assert.EqualValues(t, fixtures.AcmeID, it.WorkspaceID, "another Workspace's Templates are never listed")
	}
}

func TestGetIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	got, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID)
	assert.ErrorIs(t, err, templates.ErrNotFound)
}

func TestCreateStoresTheTemplate(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	tpl, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), templates.CreateInput{Name: "New", Subject: new("Hi")})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, tpl.WorkspaceID)
	assert.Equal(t, "Hi", tpl.Subject)
}

// Isolated per statement: a failed insert aborts the test transaction, so each
// blank-name case gets its own environment.
func TestCreateRefusesABlankName(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	before := env.DB.EmailTemplate.Query().CountX(ctx)
	_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), templates.CreateInput{Name: ""})
	require.ErrorIs(t, err, templates.ErrBlankName)
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing persisted")
}

func TestUpdateRefusesABlankNameAndLeavesTheRow(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	_, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, templates.UpdateInput{Name: new("")})
	require.ErrorIs(t, err, templates.ErrBlankName)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}

func TestUpdateChangesGivenFieldsOnly(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	got, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, templates.UpdateInput{Subject: new("Changed")})
	require.NoError(t, err)
	assert.Equal(t, "Changed", got.Subject)
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID, templates.UpdateInput{Subject: new("x")})
	assert.ErrorIs(t, err, templates.ErrNotFound)
}

// A sent message outlives its deleted Template: it stays, and no longer names the Template.
func TestDeleteLeavesTheSentMessageWithoutATemplate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	require.NoError(t, templates.New(testhelper.Renderer()).Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID))

	msg := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageWelcomeID)
	assert.Nil(t, msg.TemplateID)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
}

func TestDelete(t *testing.T) {
	env := testhelper.Setup(t)
	m := templates.New(testhelper.Renderer())
	ctx := context.Background()

	require.NoError(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID))
	require.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID), templates.ErrNotFound)
	assert.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID), templates.ErrNotFound)
}

func TestCreateRefusesAWhitespaceName(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := templates.New(testhelper.Renderer()).Create(ctx, env.DB.Scoped(fixtures.AcmeID), templates.CreateInput{Name: "   "})
	require.ErrorIs(t, err, templates.ErrBlankName)
}

func TestUpdateRefusesAWhitespaceName(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := templates.New(testhelper.Renderer()).Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, templates.UpdateInput{Name: new("  ")})
	require.ErrorIs(t, err, templates.ErrBlankName)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}
