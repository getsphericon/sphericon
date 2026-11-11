package emailtemplates_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListIsAscendingByIDAndPaged(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
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
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	got, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID)
	assert.ErrorIs(t, err, emailtemplates.ErrNotFound)
}

func TestCreateStoresTheTemplate(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	tpl, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), emailtemplates.CreateInput{Name: "New", Subject: new("Hi")})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, tpl.WorkspaceID)
	assert.Equal(t, "Hi", tpl.Subject)
}

// Isolated per statement: a failed insert aborts the test transaction, so each
// blank-name case gets its own environment.
func TestCreateRefusesABlankName(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	before := env.DB.EmailTemplate.Query().CountX(ctx)
	_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), emailtemplates.CreateInput{Name: ""})
	require.ErrorIs(t, err, validation.ErrNameBlank)
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing persisted")
}

func TestUpdateRefusesABlankNameAndLeavesTheRow(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	_, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, emailtemplates.UpdateInput{Name: patch.Set("")})
	require.ErrorIs(t, err, validation.ErrNameBlank)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}

func TestUpdateChangesGivenFieldsOnly(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	got, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, emailtemplates.UpdateInput{Revision: 1, Subject: patch.Set("Changed")})
	require.NoError(t, err)
	assert.Equal(t, "Changed", got.Subject)
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID, emailtemplates.UpdateInput{Revision: 1, Subject: patch.Set("x")})
	assert.ErrorIs(t, err, emailtemplates.ErrNotFound)
}

// Two edits made on one revision: the first is written and moves the revision by one, the second
// is refused with ErrRevisionChanged and the row stays as the first left it (ADR 0119).
func TestAnUpdateOnARevisionAlreadyMovedPastIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	read := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	first, err := m.Update(ctx, s, read.ID, emailtemplates.UpdateInput{Revision: read.Revision, Subject: patch.Set("First")})
	require.NoError(t, err)
	_, err = m.Update(ctx, s, read.ID, emailtemplates.UpdateInput{Revision: read.Revision, Subject: patch.Set("Second")})

	require.ErrorIs(t, err, emailtemplates.ErrRevisionChanged)
	assert.Equal(t, read.Revision+1, first.Revision)
	stored := env.DB.EmailTemplate.GetX(ctx, read.ID)
	assert.Equal(t, "First", stored.Subject)
	assert.Equal(t, first.Revision, stored.Revision)
}

// Every write of the row moves its revision, one made outside Module.Update too: the hook adds the
// increment to every update of the type.
func TestEveryWriteMovesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	// test writes: a write outside the module, which the hook moves the revision of, is the subject
	env.DB.EmailTemplate.UpdateOneID(before.ID).SetSubject("Elsewhere").ExecX(ctx)
	// test writes: a bulk update moves it as well
	_, err := env.DB.Scoped(fixtures.AcmeID).EmailTemplate().Update().Where(emailtemplate.ID(before.ID)).SetSubject("Again").Save(ctx)
	require.NoError(t, err)

	assert.Equal(t, before.Revision+2, env.DB.EmailTemplate.GetX(ctx, before.ID).Revision)
}

// A sent message outlives its deleted Template: it stays, and no longer names the Template.
func TestDeleteLeavesTheSentMessageWithoutATemplate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	require.NoError(t, emailtemplates.New(testhelper.Renderer(), brandkits.Brands{}).Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID))

	msg := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageWelcomeID)
	assert.Nil(t, msg.TemplateID)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
}

func TestDelete(t *testing.T) {
	env := testhelper.Setup(t)
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	ctx := context.Background()

	require.NoError(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID))
	require.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID), emailtemplates.ErrNotFound)
	assert.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateGlobexID), emailtemplates.ErrNotFound)
}

func TestCreateRefusesAWhitespaceName(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{}).Create(ctx, env.DB.Scoped(fixtures.AcmeID), emailtemplates.CreateInput{Name: "   "})
	require.ErrorIs(t, err, validation.ErrNameBlank)
}

func TestUpdateRefusesAWhitespaceName(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{}).Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.TemplateWelcomeID, emailtemplates.UpdateInput{Name: patch.Set("  ")})
	require.ErrorIs(t, err, validation.ErrNameBlank)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}

// warnedBody breaks two warning rules against Acme's saved Brand kit (Arial): a font outside it and
// an image with no alt attribute.
const warnedBody = `<p style="font-family:Georgia, serif">Sale</p><img src="https://cdn.example.com/a.png">`

func codes(fs []emailrender.Finding) []emailrender.Code {
	out := make([]emailrender.Code, len(fs))
	for i, f := range fs {
		out[i] = f.Code
	}
	return out
}

func views() *emailtemplates.Views {
	return emailtemplates.NewViews(emailtemplates.New(testhelper.Renderer(), brandkits.Brands{}))
}

// unreadBrands fails every read of the Brand kit.
type unreadBrands struct{}

func (unreadBrands) Brand(context.Context, *ent.Scoped) (*emailrender.Brand, error) {
	return nil, errors.New("brand kit read")
}

// A save through the Module stores the Template and never reads the Brand kit: the lint is the
// views', for a surface whose contract shows the warnings.
func TestASaveOutsideTheViewsIsNotLinted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := emailtemplates.New(testhelper.Renderer(), unreadBrands{})
	s := env.DB.Scoped(fixtures.AcmeID)

	tpl, err := m.Create(ctx, s, emailtemplates.CreateInput{Name: "Plain", Body: new(warnedBody)})
	require.NoError(t, err)
	updated, err := m.Update(ctx, s, tpl.ID, emailtemplates.UpdateInput{Revision: tpl.Revision, Subject: patch.Set("Hello")})
	require.NoError(t, err)

	_, err = emailtemplates.NewViews(m).Update(ctx, s, tpl.ID, emailtemplates.UpdateInput{Revision: updated.Revision, Subject: patch.Set("Hi")})
	require.ErrorContains(t, err, "brand kit read")
}

// A save stores the body as written and returns its lint against the Workspace's Brand kit
// (ADR 0040); the warnings never block it.
func TestCreateReturnsTheWarningsOfTheBodyAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	saved, err := views().Create(ctx, env.DB.Scoped(fixtures.AcmeID),
		emailtemplates.CreateInput{Name: "Warned", Subject: new("Hi"), Body: new(warnedBody)})
	require.NoError(t, err)
	assert.ElementsMatch(t, []emailrender.Code{emailrender.CodeOffBrandFont, emailrender.CodeMissingAlt}, codes(saved.Warnings))
	assert.Equal(t, warnedBody, saved.Template.Body)
}

// A Workspace with no Brand kit saved has nothing to compare a message to.
func TestCreateWithoutABrandKitSkipsBrandWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	saved, err := views().Create(ctx, env.DB.Scoped(fixtures.VandelayID),
		emailtemplates.CreateInput{Name: "Warned", Body: new(warnedBody)})
	require.NoError(t, err)
	assert.Equal(t, []emailrender.Code{emailrender.CodeMissingAlt}, codes(saved.Warnings))
}

// A write that leaves the body lints the body stored.
func TestUpdateOfTheSubjectAloneLintsTheStoredBody(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	v := views()
	s := env.DB.Scoped(fixtures.AcmeID)
	created, err := v.Create(ctx, s, emailtemplates.CreateInput{Name: "Warned", Body: new(`<img src="https://cdn.example.com/a.png">`)})
	require.NoError(t, err)

	saved, err := v.Update(ctx, s, created.Template.ID, emailtemplates.UpdateInput{Revision: created.Template.Revision, Subject: patch.Set("Hello")})
	require.NoError(t, err)
	assert.Equal(t, []emailrender.Code{emailrender.CodeMissingAlt}, codes(saved.Warnings))
}

// The lint without a save reads a body, or a document rendered as a save renders it, against the
// Brand kit; a message that cannot render is the report's Error, not a failure of the call.
func TestLintWithoutASave(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := emailtemplates.New(testhelper.Renderer(), brandkits.Brands{})
	s := env.DB.Scoped(fixtures.AcmeID)

	report, err := m.Lint(ctx, s, "Hi", new(warnedBody), nil)
	require.NoError(t, err)
	require.NoError(t, report.Error)
	assert.ElementsMatch(t, []emailrender.Code{emailrender.CodeOffBrandFont, emailrender.CodeMissingAlt}, codes(report.Warnings))

	report, err = m.Lint(ctx, s, "Hi", new("<div><span>unclosed</div>"), nil)
	require.NoError(t, err)
	require.ErrorIs(t, report.Error, emailrender.ErrInvalidContent)

	report, err = m.Lint(ctx, s, "Hi", nil, []byte(`{"not":"a document"}`))
	require.NoError(t, err)
	require.ErrorIs(t, report.Error, emailrender.ErrInvalidContent, "a document that cannot render")
	assert.Empty(t, report.Warnings)

	_, err = m.Lint(ctx, s, "Hi", nil, nil)
	require.ErrorIs(t, err, emailtemplates.ErrLintBodyOrDoc)
}
