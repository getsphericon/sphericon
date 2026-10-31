using TypeSpec.Http;

namespace Sphericon;

/** What the User does (Sign-up survey, first question) */
enum SiteSignupRole {
  founder,
  marketer,
  developer,
  product,

  /** None of the above; `roleOther` may say what */
  other,
}

/** How many people work at the User's company (second question) */
enum SiteSignupCompanySize {
  /** Only the User */
  one: "1",

  /** 2 to 10 people */
  twoToTen: "2_10",

  /** 11 to 50 people */
  elevenToFifty: "11_50",

  /** 51 to 200 people */
  fiftyOneToTwoHundred: "51_200",

  /** 201 to 1000 people */
  twoHundredOneToThousand: "201_1000",

  /** More than 1000 people */
  overThousand: "1000_plus",

  /** None of the above; `companySizeOther` may say what */
  other,
}

/** What the User mainly wants to do with the product (third question) */
enum SiteSignupGoal {
  broadcasts,
  automations,
  tracking,

  /** Moving over from another tool */
  migrating,

  /** None of the above; `mainGoalOther` may say what */
  other,
}

/** Where the User heard about the product (fourth question, optional) */
enum SiteSignupSource {
  search,
  github,
  social,
  recommendation,

  /** None of the above; `heardFromOther` may say what */
  other,
}

/** The answers to the Sign-up survey, sent once after its last screen. `role`, `companySize`
 * and `mainGoal` are required; `heardFrom` is optional. Each `…Other` text is allowed only
 * with its question's `other` choice. */
model SiteSignupSurveyInput {
  /** What the User does */
  role: SiteSignupRole;

  /** What the User does, when `role` is `other` */
  @maxLength(100)
  roleOther?: string;

  /** How many people work at the User's company */
  companySize: SiteSignupCompanySize;

  /** The company size, when `companySize` is `other` */
  @maxLength(100)
  companySizeOther?: string;

  /** What the User mainly wants to do with the product */
  mainGoal: SiteSignupGoal;

  /** The main goal, when `mainGoal` is `other` */
  @maxLength(100)
  mainGoalOther?: string;

  /** Where the User heard about the product */
  heardFrom?: SiteSignupSource;

  /** Where the User heard about the product, when `heardFrom` is `other` */
  @maxLength(100)
  heardFromOther?: string;
}

@route("/me/signup-survey")
@tag("Site User")
namespace SiteSignupSurvey {
  /** Answer the Sign-up survey. It belongs to the User, so it takes no Workspace. The answers
   * are stored and the User stops owing the survey in one transaction (204). 409 when the User
   * does not owe the survey: it was answered already, or the User was never asked (the
   * Instance has it off, or the User joined by an Invitation). 422 for a missing required
   * answer, a value outside the lists, an `…Other` text over its length or without its
   * `other` choice. Nothing is stored for a refused request. */
  @post
  op submit(@body body: SiteSignupSurveyInput):
    | NoContentResponse
    | BadRequestProblem
    | ConflictProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;
}
