using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Telegram chat a contact is reached through: its own, or that of one of its visitors. Made only by what the bot receives, so it is read here and never written. */
model SiteBindingResource {
  /** Unique identifier */
  id: EntityId;

  /** The Telegram integration (the bot) the chat is on */
  integrationId: EntityId;

  /** The bot's name as the integration is named */
  integrationName: string;

  /** The bot's `@username`, without the @-sign */
  botUsername?: string | null;

  /** When the chat was bound to its current owner */
  boundAt: Timestamp;
}

@route("/workspaces/{workspaceId}")
@tag("Site Telegram")
namespace SiteTelegram {
  /** List the Telegram chats a contact is reached through */
  @route("/contacts/{contactId}/bindings")
  @get
  @requiresPermissions(#[])
  op listBindings(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | SiteBindingResource[]
    | Problem;

  /** Issue a Telegram start link for a contact. Each call issues a new link. */
  @route("/contacts/{contactId}/telegram-start-links")
  @post
  @requiresPermissions(#[])
  op issueStartLink(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @body body: IssueTelegramStartLinkInput,
  ): TelegramStartLinkResource | Problem;
}
