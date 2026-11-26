package customfieldretyping_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/customfieldretyping"
	"github.com/getsphericon/sphericon/internal/usecase/segmentsaving"
)

// dateHasIt tests Duff's has_it with a moment test, which fits a datetime field and no string one.
// has_it is a datetime field only a notNull leaf tests, so it may become a string.
const dateHasIt = `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"` + fixtures.CustomFieldDuffHasItKey + `","test":{"kind":"moment","operator":"after","at":"2026-01-01T00:00:00Z"}}]}`

// retypeRace is a race over Duff's lock and the writers that meet in it: a retype of has_it to a
// string, and the save of a Segment and of a Rule testing has_it with a moment test.
type retypeRace struct {
	env   *testhelper.TestEnv
	scope *ent.Scoped

	retype, rename, saveSegment, saveRule testhelper.Writer
	savedID                               *int64
}

func newRetypeRace(t *testing.T) *retypeRace {
	t.Helper()
	ctx := t.Context()
	env := testhelper.Setup(t, testhelper.Committed())
	s := env.DB.Scoped(fixtures.DuffID)
	save, ruleset := testhelper.Module[*segmentsaving.UseCase](env), testhelper.Module[*rules.Module](env)
	retyping := testhelper.Module[*customfieldretyping.UseCase](env)
	r := &retypeRace{env: env, scope: s, savedID: new(int64)}
	r.rename = testhelper.Writer{Name: "rename", Run: func() error {
		_, err := retyping.Update(ctx, s, fixtures.CustomFieldDuffHasItID, contacts.CustomFieldUpdateInput{Revision: testhelper.CustomFieldRevisionIn(ctx, t, s, fixtures.CustomFieldDuffHasItID), Name: patch.Set("Has it since")})
		return err
	}}
	r.retype = testhelper.Writer{Name: "retype", Run: func() error {
		_, err := retyping.Update(ctx, s, fixtures.CustomFieldDuffHasItID, contacts.CustomFieldUpdateInput{Revision: testhelper.CustomFieldRevisionIn(ctx, t, s, fixtures.CustomFieldDuffHasItID), Type: patch.Set(string(customfield.TypeString))})
		return err
	}}
	r.saveSegment = testhelper.Writer{Name: "save", Run: func() error {
		seg, err := save.Create(ctx, s, segments.CreateInput{Name: "Has it lately", Condition: new(testhelper.SegmentCondition(t, dateHasIt))})
		if seg != nil {
			*r.savedID = seg.ID
		}
		return err
	}}
	r.saveRule = testhelper.Writer{Name: "save", Run: func() error {
		rl, err := ruleset.Create(ctx, s, rules.CreateInput{Name: "Has it lately", TriggerEvents: []string{"x"}, Condition: new(testhelper.RuleCondition(t, dateHasIt))})
		if rl != nil {
			*r.savedID = rl.ID
		}
		return err
	}}
	return r
}

func (r *retypeRace) fieldType(t *testing.T) customfield.Type {
	t.Helper()
	return r.field(t).Type
}

func (r *retypeRace) field(t *testing.T) *ent.CustomField {
	t.Helper()
	field, err := testhelper.Module[*contacts.Module](r.env).CustomField(t.Context(), r.scope, fixtures.CustomFieldDuffHasItID)
	require.NoError(t, err)
	return field
}

// Retyping a Custom field runs the Removal check and its write in one transaction under the
// Workspace's lock, which saving a Segment or a Rule takes too (ADR 0099): a Segment or a Rule saved
// first, testing the field with a leaf the new type does not take, is what the retype then finds and
// names, so the field keeps its type.
func TestRetypingACustomFieldWaitsForASaveThatTestsIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind liveuse.Kind
		save func(*retypeRace) testhelper.Writer
	}{
		{"a Segment", liveuse.Segment, func(r *retypeRace) testhelper.Writer { return r.saveSegment }},
		{"a Rule", liveuse.Rule, func(r *retypeRace) testhelper.Writer { return r.saveRule }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRetypeRace(t)

			errs := r.env.QueueBehindWorkspaceLock(t, r.scope, tc.save(r), r.retype)

			require.NoError(t, errs["save"])
			var used *liveuse.Error
			require.ErrorAs(t, errs["retype"], &used)
			kept := r.fieldType(t)
			assert.Equal(t, []liveuse.User{{Kind: tc.kind, ID: *r.savedID, Name: "Has it lately"}}, used.Users)
			assert.Equal(t, customfield.TypeDatetime, kept, "the field the save tests keeps its type")
		})
	}
}

// A retype that holds the lock first changes the type, and the Segment or the Rule saved behind it
// is checked against the new type and refused: the two never both succeed.
func TestASaveBehindARetypeIsCheckedAgainstTheNewType(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refusal error
		save    func(*retypeRace) testhelper.Writer
	}{
		{"a Segment", segments.ErrInvalidCondition, func(r *retypeRace) testhelper.Writer { return r.saveSegment }},
		{"a Rule", rules.ErrInvalid, func(r *retypeRace) testhelper.Writer { return r.saveRule }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRetypeRace(t)

			errs := r.env.QueueBehindWorkspaceLock(t, r.scope, r.retype, tc.save(r))

			require.NoError(t, errs["retype"])
			require.ErrorIs(t, errs["save"], tc.refusal)
			retyped := r.fieldType(t)
			assert.Equal(t, customfield.TypeString, retyped)
		})
	}
}

// A rename takes the same path as a retype, under the Workspace's lock (#1168): it waits for the
// lock like every other edit of a Custom field.
func TestRenamingACustomFieldWaitsForTheWorkspaceLock(t *testing.T) {
	r := newRetypeRace(t)

	errs := r.env.QueueBehindWorkspaceLock(t, r.scope, r.rename)

	require.NoError(t, errs["rename"])
	renamed := r.field(t)
	assert.Equal(t, "Has it since", renamed.Name)
}
