//go:build e2e

package e2e

import (
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// ImportCSV imports a CSV file of Contacts through the app as the Workspace's owner does (ADR
// 0138): it uploads the file, saves the mapping the app proposes from its header with tag given to
// every row, runs the import and waits until its job is done. It returns the import as it ended.
func (w *Workspace) ImportCSV(file, tag string) *siteapi.SiteContactImport {
	w.t.Helper()
	return w.importCSV(file, func(m *siteapi.ContactImportMappingInput) { m.Tag = siteapi.NewNilName(siteapi.Name(tag)) })
}

// ImportCSVAsNewSignUps is ImportCSV with "treat them as new sign-ups" ticked: the import's
// Events start what a sign-up starts.
func (w *Workspace) ImportCSVAsNewSignUps(file, tag string) *siteapi.SiteContactImport {
	w.t.Helper()
	return w.importCSV(file, func(m *siteapi.ContactImportMappingInput) {
		m.Tag = siteapi.NewNilName(siteapi.Name(tag))
		m.AsNewSignUps = true
	})
}

// importCSV uploads file, saves the mapping the app proposes as mapped changes it, runs the import
// and waits until its job is done.
func (w *Workspace) importCSV(file string, mapped func(*siteapi.ContactImportMappingInput)) *siteapi.SiteContactImport {
	w.t.Helper()
	ctx := w.t.Context()
	ws := siteapi.WorkspaceId(w.publicID)
	uploaded, err := w.site.SiteContactImportsUpload(ctx, siteapi.SiteContactImportsUploadReq{Data: strings.NewReader(file)},
		siteapi.SiteContactImportsUploadParams{WorkspaceId: ws})
	require.NoError(w.t, err, "upload the file")

	raw, err := uploaded.Mapping.MarshalJSON()
	require.NoError(w.t, err)
	var mapping siteapi.ContactImportMappingInput
	require.NoError(w.t, mapping.UnmarshalJSON(raw), "the proposal as a request saves it")
	mapped(&mapping)
	_, err = w.site.SiteContactImportsSaveMapping(ctx, &mapping, siteapi.SiteContactImportsSaveMappingParams{WorkspaceId: ws, ImportId: uploaded.ID})
	require.NoError(w.t, err, "save the mapping")
	_, err = w.site.SiteContactImportsRun(ctx, siteapi.SiteContactImportsRunParams{WorkspaceId: ws, ImportId: uploaded.ID})
	require.NoError(w.t, err, "run the import")

	var done *siteapi.SiteContactImport
	require.Eventually(w.t, func() bool {
		imp, err := w.site.SiteContactImportsGet(ctx, siteapi.SiteContactImportsGetParams{WorkspaceId: ws, ImportId: uploaded.ID})
		done = imp
		return err == nil && imp.State == siteapi.ContactImportStateDone
	}, EmailTimeout, 100*time.Millisecond, "the import of %s never ended done", w)
	return done
}

// RequireImported asserts the Contact with this email exists, came from an import (its source) and
// carries the Tags.
func (w *Workspace) RequireImported(email string, tags ...string) {
	w.t.Helper()
	doc, found := w.contactExport(email)
	require.True(w.t, found, "export of %s", email)
	require.Equal(w.t, externalapi.ContactExportSourceImport, doc.Contact.Source, "%s came from the import", email)
	names := lo.Map(doc.Tags, func(t externalapi.ContactExportTag, _ int) string { return t.Name })
	require.Subset(w.t, names, tags, "%s carries the import's Tags", email)
}
