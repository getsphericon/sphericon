package contactsexport_test

import (
	"bytes"
	"encoding/csv"
	"slices"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactsexport"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// session stands for the session token of the request that asks for an export and follows it.
const session = "a session token"

// export asks for the export of the Workspace's Contacts, or a Segment's, follows its link and reads
// the file.
func export(t *testing.T, env *testhelper.TestEnv, s *ent.Scoped, segmentID *int64, cols ...contactsexport.Column) [][]string {
	t.Helper()
	m := testhelper.Module[*contactsexport.Module](env)
	link, err := m.Link(t.Context(), s, contactsexport.LinkInput{SegmentID: segmentID, Columns: cols, Session: session})
	require.NoError(t, err)
	d, err := m.Open(t.Context(), s, contactsexport.OpenInput{Token: link.Token, Session: session})
	require.NoError(t, err)
	var file bytes.Buffer
	for chunk, err := range d.Chunks {
		require.NoError(t, err)
		file.Write(chunk)
	}
	body, found := strings.CutPrefix(file.String(), "\xEF\xBB\xBF")
	require.True(t, found, "the file begins with a UTF-8 BOM")
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	require.NoError(t, err)
	return rows
}

// rowOf is the row whose email column holds the address, by header.
func rowOf(t *testing.T, rows [][]string, email string) map[string]string {
	t.Helper()
	header := rows[0]
	at := slices.Index(header, string(contactsexport.ColumnEmail))
	row, found := lo.Find(rows[1:], func(r []string) bool { return r[at] == email })
	require.True(t, found, "a row of %s", email)
	return lo.SliceToMap(lo.Range(len(header)), func(i int) (string, string) { return header[i], row[i] })
}

// A Channel is six columns, each headed by the Channel and its part.
func TestAChannelIsSixColumnsOfItsState(t *testing.T) {
	env := testhelper.Setup(t)

	rows := export(t, env, env.DB.Scoped(fixtures.TyrellID), nil, contactsexport.ColumnEmail, contactsexport.ColumnSMSChannel)

	assert.Equal(t, []string{
		"email", "sms.state", "sms.optedOutScopes", "sms.optedOutAt", "sms.suppressionReason", "sms.confirmation", "sms.confirmedAt",
	}, rows[0])
	assert.Len(t, rows, 1+7, "the header and each Contact of the matrix")
}

// A Destination opted out of Broadcasts and suppressed shows both: its state is the Suppression,
// and its scope and the time it stopped stand beside it.
func TestADestinationOptedOutAndSuppressedShowsBoth(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.TyrellID)
	moved, err := s.Unsubscribe().Update().
		Where(unsubscribe.ChannelEQ(channels.Email), unsubscribe.DestinationEQ(fixtures.TyrellMatrixUnsubBroadcastsEmail)).
		SetDestination(fixtures.TyrellMatrixSuppressedEmail).
		Save(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, moved)

	rows := export(t, env, s, nil, contactsexport.ColumnEmail, contactsexport.ColumnEmailChannel)

	assert.Equal(t, map[string]string{
		"email":                   fixtures.TyrellMatrixSuppressedEmail,
		"email.state":             "suppressed",
		"email.optedOutScopes":    "broadcasts",
		"email.optedOutAt":        "2026-01-01T00:00:00Z",
		"email.suppressionReason": "bounce",
		"email.confirmation":      "",
		"email.confirmedAt":       "",
	}, rowOf(t, rows, fixtures.TyrellMatrixSuppressedEmail))
	assert.Equal(t, "may_receive", rowOf(t, rows, fixtures.TyrellMatrixCleanEmail)["email.state"])
	confirmed := rowOf(t, rows, fixtures.TyrellMatrixConfirmedSuppressedEmail)
	assert.Equal(t, "double_opt_in", confirmed["email.confirmation"])
	assert.Equal(t, "2026-01-01T00:00:00Z", confirmed["email.confirmedAt"])
}

// A Segment's export holds its members only.
func TestASegmentsExportHoldsItsMembers(t *testing.T) {
	env := testhelper.Setup(t)
	segment := int64(fixtures.SegmentTyrellTextableID)

	rows := export(t, env, env.DB.Scoped(fixtures.TyrellID), &segment, contactsexport.ColumnEmail)

	assert.Equal(t, [][]string{{"email"}, {fixtures.TyrellMatrixConfirmedEmail}}, rows)
}

// A cell a spreadsheet would run is written with a leading apostrophe.
func TestACellASpreadsheetWouldRunIsNeutralised(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.TyrellID)
	err := s.Contact().UpdateOneID(fixtures.TyrellMatrixCleanID).SetFirstName(`=HYPERLINK("http://evil.test")`).Exec(t.Context())
	require.NoError(t, err)

	rows := export(t, env, s, nil, contactsexport.ColumnEmail, contactsexport.ColumnFirstName)

	assert.Equal(t, `'=HYPERLINK("http://evil.test")`, rowOf(t, rows, fixtures.TyrellMatrixCleanEmail)["firstName"])
}
