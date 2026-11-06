package signupsurvey

import (
	"cmp"
	"context"
	"slices"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
)

// Reader is the read side of the Sign-up survey, for the Staff console: it holds the raw client
// because the answers belong to Users, not to a Workspace. It writes nothing.
type Reader struct {
	client *ent.Client
}

// NewReader builds the reader over the raw client.
func NewReader(client *ent.Client) *Reader {
	return &Reader{client: client}
}

// ChoiceCount is how many Users chose one answer of a question.
type ChoiceCount struct {
	Choice string
	Count  int
}

// OtherCount is an `other` text and how many Users wrote it.
type OtherCount struct {
	Text  string
	Count int
}

// Tally is the answers of one question: the choices somebody made (none with a zero count),
// in alphabetical order, and the `other` texts, most frequent first.
type Tally struct {
	Choices []ChoiceCount
	Others  []OtherCount
}

// Counts are the answers of every User who answered, per question. A User who has not
// answered is in no count.
type Counts struct {
	Responses   int
	Role        Tally
	CompanySize Tally
	MainGoal    Tally
	HeardFrom   Tally
}

// Counts tallies every stored answer, grouped and counted by the database.
func (r *Reader) Counts(ctx context.Context) (Counts, error) {
	responses, err := r.client.SignupSurveyResponse.Query().Count(ctx)
	if err != nil {
		return Counts{}, err
	}
	out := Counts{Responses: responses}
	for _, q := range []struct {
		tally         *Tally
		choice, other string
	}{
		{&out.Role, signupsurveyresponse.FieldRole, signupsurveyresponse.FieldRoleOtherText},
		{&out.CompanySize, signupsurveyresponse.FieldCompanySize, signupsurveyresponse.FieldCompanySizeOtherText},
		{&out.MainGoal, signupsurveyresponse.FieldMainGoal, signupsurveyresponse.FieldMainGoalOtherText},
		{&out.HeardFrom, signupsurveyresponse.FieldHeardFrom, signupsurveyresponse.FieldHeardFromOtherText},
	} {
		choices, err := r.grouped(ctx, q.choice)
		if err != nil {
			return Counts{}, err
		}
		others, err := r.grouped(ctx, q.other)
		if err != nil {
			return Counts{}, err
		}
		*q.tally = Tally{Choices: choiceCounts(choices), Others: otherCounts(others)}
	}
	return out, nil
}

// group is one value of a column and how many rows hold it.
type group struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// grouped counts the rows per value of a nullable-or-not column; a NULL (the optional question
// skipped, an `other` text not written) is in no group.
func (r *Reader) grouped(ctx context.Context, column string) ([]group, error) {
	var rows []group
	err := r.client.SignupSurveyResponse.Query().
		Modify(func(s *sql.Selector) {
			col := s.C(column)
			s.Where(sql.NotNull(col)).
				Select(sql.As(col, "value"), sql.As(sql.Count("*"), "count")).
				GroupBy(col)
		}).
		Scan(ctx, &rows)
	return rows, err
}

// choiceCounts orders the choices by name so a response is stable.
func choiceCounts(rows []group) []ChoiceCount {
	out := make([]ChoiceCount, 0, len(rows))
	for _, g := range rows {
		out = append(out, ChoiceCount{Choice: g.Value, Count: g.Count})
	}
	slices.SortFunc(out, func(a, b ChoiceCount) int { return cmp.Compare(a.Choice, b.Choice) })
	return out
}

// otherCounts orders the texts by count, most frequent first, then by text.
func otherCounts(rows []group) []OtherCount {
	out := make([]OtherCount, 0, len(rows))
	for _, g := range rows {
		out = append(out, OtherCount{Text: g.Value, Count: g.Count})
	}
	slices.SortFunc(out, func(a, b OtherCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Text, b.Text))
	})
	return out
}

// ForUser is the answers of one User; ent.IsNotFound when the User has not answered (or does
// not exist).
func (r *Reader) ForUser(ctx context.Context, userID int64) (*ent.SignupSurveyResponse, error) {
	return r.client.SignupSurveyResponse.Query().
		Where(signupsurveyresponse.UserID(userID)).
		Only(ctx)
}
