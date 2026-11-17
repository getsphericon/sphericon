import "../../common/signup-survey.tsp";

using TypeSpec.Http;

namespace Sphericon;

/** The answers to the Sign-up survey, sent once after its last screen. `role`, `companySize`
 * and `mainGoal` are required; `heardFrom` is optional. Each `…Other` text is allowed only
 * with its question's `other` choice. */
model SiteSignupSurveyInput {
  /** What the User does */
  role: SignupRole;

  /** What the User does, when `role` is `other` */
  @maxLength(100)
  roleOther?: string;

  /** How many people work at the User's company */
  companySize: SignupCompanySize;

  /** The company size, when `companySize` is `other` */
  @maxLength(100)
  companySizeOther?: string;

  /** What the User mainly wants to do with the product */
  mainGoal: SignupGoal;

  /** The main goal, when `mainGoal` is `other` */
  @maxLength(100)
  mainGoalOther?: string;

  /** Where the User heard about the product */
  heardFrom?: SignupSource;

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
  op submit(@body body: SiteSignupSurveyInput): NoContentResponse | Problem;
}
