using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

@tag("Telegram")
namespace TelegramStartLinks {
  /** Issue a start link for a contact: `https://t.me/<bot>?start=<token>`. The chat that starts with it binds to the contact, once. Each call issues a new link. */
  @route("/contacts/{contactId}/telegram-start-links")
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op issue(@path contactId: EntityId, @body body: IssueTelegramStartLinkInput):
    | TelegramStartLinkResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
