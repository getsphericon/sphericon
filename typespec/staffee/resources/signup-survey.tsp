using TypeSpec.Http;

namespace Sphericon;

/** What the User does (Sign-up survey, first question) */
enum StaffSignupRole {
  founder,
  marketer,
  developer,
  product,
  other,
}

/** How many people work at the User's company (second question) */
enum StaffSignupCompanySize {
  one: "1",
  twoToTen: "2_10",
  elevenToFifty: "11_50",
  fiftyOneToTwoHundred: "51_200",
  twoHundredOneToThousand: "201_1000",
  overThousand: "1000_plus",
  other,
}

/** What the User mainly wants to do with the product (third question) */
enum StaffSignupGoal {
  broadcasts,
  automations,
  tracking,
  migrating,
  other,
}

/** Where the User heard about the product (fourth question, optional) */
enum StaffSignupSource {
  search,
  github,
  social,
  recommendation,
  other,
}

/** One User's answers to the Sign-up survey. A User who has not answered has none. */
model StaffSignupSurveyResponseResource {
  /** The answering User's id */
  userId: EntityId;

  /** What the User does */
  role: StaffSignupRole;

  /** What the User does, when `role` is `other` */
  roleOtherText?: string | null;

  /** How many people work at the User's company */
  companySize: StaffSignupCompanySize;

  /** The company size, when `companySize` is `other` */
  companySizeOtherText?: string | null;

  /** What the User mainly wants to do with the product */
  mainGoal: StaffSignupGoal;

  /** The main goal, when `mainGoal` is `other` */
  mainGoalOtherText?: string | null;

  /** Where the User heard about the product; absent when the optional question was skipped */
  heardFrom?: StaffSignupSource | null;

  /** Where the User heard about the product, when `heardFrom` is `other` */
  heardFromOtherText?: string | null;

  /** When the User answered */
  answeredAt: Timestamp;
}

/** How many Users chose a role */
model StaffSignupRoleCount {
  choice: StaffSignupRole;
  count: int32;
}

/** How many Users chose a company size */
model StaffSignupCompanySizeCount {
  choice: StaffSignupCompanySize;
  count: int32;
}

/** How many Users chose a main goal */
model StaffSignupGoalCount {
  choice: StaffSignupGoal;
  count: int32;
}

/** How many Users chose a source */
model StaffSignupSourceCount {
  choice: StaffSignupSource;
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
  op counts(): StaffSignupSurveyCounts | UnauthorizedProblem;

  /** One User's answers. 404 when the User is unknown or has not answered the survey. */
  @get
  @route("/users/{userId}")
  op get(@path userId: EntityId):
    | StaffSignupSurveyResponseResource
    | UnauthorizedProblem
    | NotFoundProblem;
}
