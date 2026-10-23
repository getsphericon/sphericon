package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// otherMaxLen is the length of an `other` text, as the TypeSpec contract declares it.
const otherMaxLen = 100

// otherColumn gives each `other` text the same cap in the table, so the database refuses a longer one.
var otherColumn = map[string]string{dialect.Postgres: "varchar(100)"}

// SignupSurveyResponse is one User's answers to the Sign-up survey: one row per User, created
// when the survey is submitted and removed with the User (the foreign key cascades, ADR 0050).
// It belongs to a User, not a Workspace, so it has no scoped wrapper; only
// internal/signupsurvey writes it. Each question has an `other` choice with a short text.
// heard_from is optional (null when it was skipped); the other answers are required.
type SignupSurveyResponse struct {
	ent.Schema
}

func (SignupSurveyResponse) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("signup_survey_responses", SignupSurveyResponse{}.Fields()),
	}
}

func (SignupSurveyResponse) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SignupSurveyResponse) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable().
			Unique(),
		field.Enum("role").
			Values("founder", "marketer", "developer", "product", "other").
			Immutable(),
		field.String("role_other_text").
			MaxLen(otherMaxLen).
			SchemaType(otherColumn).
			Optional().
			Nillable().
			Immutable(),
		field.Enum("company_size").
			Values("1", "2_10", "11_50", "51_200", "201_1000", "1000_plus", "other").
			Immutable(),
		field.String("company_size_other_text").
			MaxLen(otherMaxLen).
			SchemaType(otherColumn).
			Optional().
			Nillable().
			Immutable(),
		field.Enum("main_goal").
			Values("broadcasts", "automations", "tracking", "migrating", "other").
			Immutable(),
		field.String("main_goal_other_text").
			MaxLen(otherMaxLen).
			SchemaType(otherColumn).
			Optional().
			Nillable().
			Immutable(),
		field.Enum("heard_from").
			Values("search", "github", "social", "recommendation", "other").
			Optional().
			Nillable().
			Immutable(),
		field.String("heard_from_other_text").
			MaxLen(otherMaxLen).
			SchemaType(otherColumn).
			Optional().
			Nillable().
			Immutable(),
	}
}

func (SignupSurveyResponse) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("signup_survey_response").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}
