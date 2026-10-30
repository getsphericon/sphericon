using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The surface that asked for the send a Captured message belongs to */
enum CapturedMessageSurface {
  broadcast,
  automation,
  transactional,

  /** A Test send of a Broadcast: it has no Outbound message */
  test,
}

/** One header line of a Captured message */
model CapturedMessageHeader {
  @extension("x-untrusted", true)
  name: string;

  @extension("x-untrusted", true)
  value: string;
}

/** What a Captured message list shows of each message: everything but the content */
model CapturedMessageFields {
  /** The Test inbox (Integration) that captured the message */
  integrationId: EntityId;

  /** The Outbound message of the send; null for a Test send, which records none */
  outboundMessageId: EntityId | null;

  /** The surface that asked for the send */
  surface: CapturedMessageSurface;

  /** The Sending source of a Broadcast or Automation send; null for a Transactional or Test send */
  sendingSource: string | null;

  /** Normalized (lower-cased) recipient address */
  recipient: string;

  /** The From address as it was, not rewritten */
  from: string;

  /** The From name; null when the message has none */
  @extension("x-untrusted", true)
  fromName: string | null;

  /** The subject */
  @extension("x-untrusted", true)
  subject: string;

  /** True when the From is not on a verified Sending domain: the message would not pass the Sending-domain gate at a provider that delivers outside */
  fromUnverified: boolean;

  /** When the message was captured */
  createdAt: Timestamp;
}

/** A Captured message as the list shows it: without the content */
model CapturedMessageSummary {
  /** Unique identifier */
  id: EntityId;

  ...CapturedMessageFields;
}

/** A message a Test inbox kept instead of sending it out: what the recipient would have received */
@resource("captured-messages")
@segment("captured-messages")
model CapturedMessageResource {
  /** Unique identifier */
  @key
  id: EntityId;

  ...CapturedMessageFields;

  /** The HTML part */
  @extension("x-untrusted", true)
  html: string;

  /** The text part */
  @extension("x-untrusted", true)
  text: string;

  /** The header lines the message would carry on the wire, in order */
  headers: CapturedMessageHeader[];

  /** The URLs of the links of the HTML part, tracker links as the recipient would get them */
  @extension("x-untrusted", true)
  links: string[];
}

/** Captured messages: what the Workspace's Test inbox kept instead of sending out (ADR 0058). Reading needs the `captured_messages:read` scope, deleting `captured_messages:write`. */
@tag("Captured Messages")
interface CapturedMessages {
  /** List the Captured messages, newest first, without their content (read one for it) */
  @listsResource(CapturedMessageResource)
  @get
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.capturedMessagesRead])
  list(...ResourceCollectionParameters<CapturedMessageResource>, ...PageQuery):
    | Page<CapturedMessageSummary>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

  /** Read one Captured message in full: headers, HTML, text and links */
  @readsResource(CapturedMessageResource)
  @get
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.capturedMessagesRead])
  get(...ResourceParameters<CapturedMessageResource>):
    | CapturedMessageResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Delete one Captured message */
  @deletesResource(CapturedMessageResource)
  @delete
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.capturedMessagesWrite])
  delete(...ResourceParameters<CapturedMessageResource>):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Delete every Captured message of the Workspace. Needs a person: hidden from MCP. */
  @collectionAction(CapturedMessageResource, "all")
  @delete
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.capturedMessagesWrite])
  deleteAll(...ResourceCollectionParameters<CapturedMessageResource>):
    | NoContentResponse
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}
