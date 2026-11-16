import "../../common/signup-survey.tsp";

using TypeSpec.Http;

namespace Sphericon;

/** One User's answers to the Sign-up survey. A User who has not answered has none. */
model StaffSignupSurveyResponseResource {
  /** The answering User's id */
  userId: EntityId;

  /** What the User does */
  role: SignupRole;

  /** What the User does, when `role` is `other` */
  roleOtherText?: string | null;

  /** How many people work at the User's company */
  companySize: SignupCompanySize;

  /** The company size, when `companySize` is `other` */
  companySizeOtherText?: string | null;

  /** What the User mainly wants to do with the product */
  mainGoal: SignupGoal;

  /** The main goal, when `mainGoal` is `other` */
  mainGoalOtherText?: string | null;

  /** Where the User heard about the product; absent when the optional question was skipped */
  heardFrom?: SignupSource | null;

  /** Where the User heard about the product, when `heardFrom` is `other` */
  heardFromOtherText?: string | null;

  /** When the User answered */
  answeredAt: Timestamp;
}

/** How many Users chose a role */
model StaffSignupRoleCount {
  choice: SignupRole;
  count: int32;
}

/** How many Users chose a company size */
model StaffSignupCompanySizeCount {
  choice: SignupCompanySize;
  count: int32;
}

/** How many Users chose a main goal */
model StaffSignupGoalCount {
  choice: SignupGoal;
  count: int32;
}

/** How many Users chose a source */
model StaffSignupSourceCount {
  choice: SignupSource;
  count: int32;
}

/** An `other` text and how many Users wrote it */
model StaffSignupOtherTextCount {
  text: string;
  count: int32;
}

/** The Sign-up survey answers of every User who answered, counted per question. A choice nobody
 * made is absent from its list; a question's `other` texts are listed with their counts, most
 * frequent first. */
model StaffSignupSurveyCounts {
  /** How many Users answered the survey */
  responses: int32;

  role: StaffSignupRoleCount[];
  roleOtherTexts: StaffSignupOtherTextCount[];
  companySize: StaffSignupCompanySizeCount[];
  companySizeOtherTexts: StaffSignupOtherTextCount[];
  mainGoal: StaffSignupGoalCount[];
  mainGoalOtherTexts: StaffSignupOtherTextCount[];

  /** Only the Users who answered the optional question are counted */
  heardFrom: StaffSignupSourceCount[];

  heardFromOtherTexts: StaffSignupOtherTextCount[];
}

@tag("StaffMember Sign-up survey")
@route("/signup-survey")
namespace StaffSignupSurvey {
  /** The Sign-up survey answers, counted per question. Read only. */
  @get
  op counts(): StaffSignupSurveyCounts | Problem;

  /** One User's answers. 404 when the User is unknown or has not answered the survey. */
  @get
  @route("/users/{userId}")
  op get(@path userId: EntityId): StaffSignupSurveyResponseResource | Problem;
}
