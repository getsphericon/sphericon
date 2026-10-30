import "../common/scopes.tsp";

namespace Sphericon;

/** The scopes a User may give a credential of their own: a Connection by consenting, a Personal
 * token by minting it (ADR 0061). The authorization server advertises these and no other, and the
 * Personal token form offers them. Each still needs the Permission its enum declares in
 * `x-grant-permissions`; the token-only scopes are never a User's to give. Only /site names it,
 * so it is declared here and not with the external vocabulary. */
enum PersonalScope {
  ...GrantableScope,
  ...SendScope,
  ...SetupScope,
}
