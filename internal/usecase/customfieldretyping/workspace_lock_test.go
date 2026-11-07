package customfieldretyping_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/customfieldretyping"
)

// dateHasIt tests Duff's has_it with a date leaf, which fits a datetime field and no string one.
// has_it is a datetime field only a notNull leaf tests, so it may become a string.
const dateHasIt = `{"kind":"group","combinator":"and","rules":[{"kind":"date","field":"custom:` + fixtures.CustomFieldDuffHasItKey + `","operator":"after","at":"2026-01-01T00:00:00Z"}]}`

// retypeRace is a race over Duff's lock and the writers that meet in it: a retype of has_it to a
// string, and the save of a Segment and of a Rule testing has_it with a date leaf.
type retypeRace struct {
	*testhelper.WorkspaceRace

	retype, saveSegment, saveRule testhelper.Writer
	savedID                       *int64
}

func newRetypeRace(t *testing.T) *retypeRace {
	t.Helper()
	ctx := context.Background()
	race := testhelper.NewWorkspaceRace(t, fixtures.DuffID)
	s, bus := race.Scope, race.Bus
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	segs, ruleset, retyping := segments.New(bus), rules.New(cipher, bus), customfieldretyping.New(bus)
	r := &retypeRace{WorkspaceRace: race, savedID: new(int64)}
	r.retype = testhelper.Writer{Name: "retype", Run: func() error {
		_, err := retyping.Update(ctx, s, fixtures.CustomFieldDuffHasItID, contacts.CustomFieldUpdateInput{Type: patch.Set(string(customfield.TypeString))})
		return err
	}}
	r.saveSegment = testhelper.Writer{Name: "save", Run: func() error {
		seg, err := segs.Create(ctx, s, segments.CreateInput{Name: "Has it lately", Condition: new(dateHasIt)})
		if seg != nil {
			*r.savedID = seg.ID
		}
		return err
	}}
	r.saveRule = testhelper.Writer{Name: "save", Run: func() error {
		rl, err := ruleset.Create(ctx, s, rules.CreateInput{Name: "Has it lately", TriggerEvents: []string{"x"}, Condition: new(dateHasIt)})
		if rl != nil {
			*r.savedID = rl.ID
		}
		return err
	}}
	return r
}

func (r *retypeRace) fieldType(t *testing.T) customfield.Type {
	t.Helper()
	field, err := contacts.CustomField(context.Background(), r.Scope, fixtures.CustomFieldDuffHasItID)
	require.NoError(t, err)
	return field.Type
}

// Retyping a Custom field runs its Live use check and its write in one transaction under the
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

			errs := r.Queue(t, tc.save(r), r.retype)

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

			errs := r.Queue(t, r.retype, tc.save(r))

			require.NoError(t, errs["retype"])
			require.ErrorIs(t, errs["save"], tc.refusal)
			retyped := r.fieldType(t)
			assert.Equal(t, customfield.TypeString, retyped)
		})
	}
}
