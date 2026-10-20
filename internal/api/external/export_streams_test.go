package external_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/exportstream"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// exportPage is one answer of an Export stream as a consumer sees it: the header, the lines.
type exportPage struct {
	cursor string
	rows   []map[string]any
}

// pageThrough reads a stream in pages of limit until a page comes back empty (or maxPages have
// been read) and returns every row id in the order served, with the cursor the last page left.
func pageThrough(read func(cursor string, limit int32) exportPage, limit int32, maxPages int) ([]int64, string) {
	var seen []int64
	cursor := ""
	for range maxPages {
		page := read(cursor, limit)
		if len(page.rows) == 0 {
			break
		}
		seen = append(seen, page.ids()...)
		cursor = page.cursor
	}
	return seen, cursor
}

func (p exportPage) ids() []int64 {
	return lo.Map(p.rows, func(r map[string]any, _ int) int64 {
		id, err := strconv.ParseInt(r["id"].(string), 10, 64)
		if err != nil {
			panic(err)
		}
		return id
	})
}

func (p exportPage) row(id int64) map[string]any {
	for _, r := range p.rows {
		if r["id"] == strconv.FormatInt(id, 10) {
			return r
		}
	}
	return nil
}

// readExport reads one page of the Events stream and checks every line against the
// TypeSpec row schema, so a stream that drifts from its contract fails every test here.
func readExportEvents(t *testing.T, c *externalapi.Client, cursor string, limit int32) exportPage {
	t.Helper()
	params := externalapi.ExportEventsParams{}
	if cursor != "" {
		params.Cursor = externalapi.NewOptString(cursor)
	}
	if limit > 0 {
		params.Limit = externalapi.NewOptInt32(limit)
	}
	res, err := c.ExportEvents(context.Background(), params)
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.ExportEventsOKHeaders)
	require.Truef(t, isOK, "got %T", res)
	body, err := io.ReadAll(ok.Response)
	require.NoError(t, err)

	page := exportPage{cursor: ok.XNextCursor}
	for line := range bytes.SplitSeq(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		require.NoError(t, exportstream.ValidateRow(exportstream.EventsSchema, line), string(line))
		var row map[string]any
		require.NoError(t, json.Unmarshal(line, &row))
		page.rows = append(page.rows, row)
	}
	return page
}

func exportClient(t *testing.T, env *testhelper.TestEnv, workspaceID int64) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearerFor(t, workspaceID, "export:read"))
}

func TestExportEventsReadsTheWholeHistoryFromAnEmptyCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	page := readExportEvents(t, c, "", 0)

	require.NotEmpty(t, page.rows)
	require.NotEmpty(t, page.cursor)
	custom := page.row(fixtures.EventErasableCustomID)
	require.NotNil(t, custom)
	assert.Equal(t, fixtures.EventErasableCustomAction, custom["action"])
	assert.Equal(t, "erin@initech.test", custom["email"])
	anonymous := page.row(fixtures.EventErasableAnonymousID)
	require.NotNil(t, anonymous)
	assert.Nil(t, anonymous["contact_id"])
	assert.Equal(t, "vis-erasable-laptop", anonymous["visitor_id"])
}

func TestExportEventsEmptyPageReturnsTheSameCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)
	first := readExportEvents(t, c, "", 0)

	caughtUp := readExportEvents(t, c, first.cursor, 0)

	assert.Empty(t, caughtUp.rows)
	assert.Equal(t, first.cursor, caughtUp.cursor)
}

func TestExportEventsPagingLosesNothingAcrossEqualUpdatedAt(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)
	whole := readExportEvents(t, c, "", 0)
	require.Greater(t, len(whole.rows), 3, "the fixture Workspace has several Events")
	first, last := whole.rows[0]["updated_at"], whole.rows[len(whole.rows)-1]["updated_at"]
	require.Equal(t, first, last, "the fixture rows share one updated_at, so every page edge is a tie")

	seen, _ := pageThrough(func(cursor string, limit int32) exportPage { return readExportEvents(t, c, cursor, limit) }, 3, len(whole.rows)+1)

	assert.Equal(t, whole.ids(), seen, "every row exactly once, in order")
}

func TestExportEventsDeliversAStitchedAnonymousEventAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := exportClient(t, env, fixtures.InitechID)
	before := readExportEvents(t, c, "", 0)
	require.Nil(t, before.row(fixtures.EventErasableAnonymousID)["contact_id"])

	_, err := env.CollectWithKey(t, fixtures.InitechCollectKey).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "vis-erasable-laptop",
		Email:     collectapi.NewOptNilEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)

	after := readExportEvents(t, c, before.cursor, 0)
	stitched := after.row(fixtures.EventErasableAnonymousID)
	require.NotNil(t, stitched, "the stitched Event comes again from the cursor taken before")
	assert.NotNil(t, stitched["contact_id"])
}

func TestExportEventsDeliversAnAnonymizedSystemEventAgain(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)
	before := readExportEvents(t, c, "", 0)
	require.Equal(t, fixtures.EventErasableSentEmail, before.row(fixtures.EventErasableSentID)["email"])

	eraseErasable(t, env)

	after := readExportEvents(t, c, before.cursor, 0)
	sent := after.row(fixtures.EventErasableSentID)
	require.NotNil(t, sent, "the anonymized email.* Event comes again")
	assert.Nil(t, sent["email"])
	assert.Nil(t, after.row(fixtures.EventErasableCustomID), "a deleted Event is not in the stream; the Erasures stream announces it")
}

func TestExportEventsNeedsTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]string{{}, {"contacts:read"}, {"contacts:read", "events:read", "audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
		res, err := c.ExportEvents(context.Background(), externalapi.ExportEventsParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.ExportEventsUnauthorized{}, res, "scopes %v are refused", scopes)
	}
}

func TestExportEventsAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportClient(t, env, fixtures.InitechID)

	res, err := c.ExportEvents(context.Background(), externalapi.ExportEventsParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportEventsNotFound{}, res)
}

func TestExportEventsNeverReadsAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	globex := readExportEvents(t, exportClient(t, env, fixtures.GlobexID), "", 0)
	initech := readExportEvents(t, exportClient(t, env, fixtures.InitechID), "", 0)

	require.NotNil(t, globex.row(fixtures.EventGlobexID))
	assert.Nil(t, initech.row(fixtures.EventGlobexID))
	assert.Nil(t, globex.row(fixtures.EventErasableCustomID))
}

func TestExportEventsOfASuspendedWorkspaceStillRead(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.SuspendWorkspace(context.Background(), env.Bus, fixtures.InitechID, suspension.CLI, "abuse")
	require.NoError(t, err)

	page := readExportEvents(t, exportClient(t, env, fixtures.InitechID), "", 0)

	assert.NotEmpty(t, page.rows)
}

func TestExportEventsOfADeletedWorkspaceDoNotAuthenticate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := exportClient(t, env, fixtures.InitechID)
	readExportEvents(t, c, "", 0)
	env.DB.Workspace.UpdateOneID(fixtures.InitechID).SetDeletedAt(time.Now()).ExecX(ctx)

	res, err := c.ExportEvents(ctx, externalapi.ExportEventsParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportEventsUnauthorized{}, res)
}

func TestExportEventsRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	res, err := c.ExportEvents(context.Background(), externalapi.ExportEventsParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportEventsBadRequest{}, res)
}
