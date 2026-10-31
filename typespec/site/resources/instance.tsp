using TypeSpec.Http;

namespace Sphericon;

/** What the Release check found (ADR 0032): the one part of the Instance's state a Workspace Owner
 * sees. The Instance's settings are the Staff console's (ADR 0063). */
model SiteInstanceRelease {
  /** The newest stable release (a bare version such as `0.4.0`) when the Release check found
   * one newer than the running version; absent otherwise, and always absent for a build whose
   * version is not a release or in the saas Mode, which makes no Release check. */
  latestRelease?: string;
}

@route("/instance")
@tag("Site Instance")
namespace SiteInstance {
  /** The newer release the Release check found, for its banner. Refused (403) to a User who holds
   * `release:read` in no live Workspace. */
  @get
  @route("/release")
  op getRelease(): SiteInstanceRelease | ForbiddenProblem;
}
