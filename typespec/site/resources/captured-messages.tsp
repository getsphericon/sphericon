using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The surface that asked for the send a Captured message belongs to */
enum SiteCapturedMessageSurface {
  broadcast,
  automation,
  transactional,

  /** A Test send of a Broadcast: it has no Outbound message */
  test,
}

/** One header line of a Captured message */
model SiteCapturedMessageHeader {
  name: string;
  value: string;
}

/** What a Captured message list shows of each message: everything but the content */
model SiteCapturedMessageFields {
  /** The Test inbox (Integration) that captured the message */
  integrationId: EntityId;

  /** The Outbound message of the send; null for a Test send, which records none */
  outboundMessageId: EntityId | null;

  /** The surface that asked for the send */
  surface: SiteCapturedMessageSurface;

  /** The Sending source of a Broadcast or Automation send; null for a Transactional or Test send */
  sendingSource: string | null;

  /** Normalized (lower-cased) recipient address */
  recipient: string;

  /** The From address as it was, not rewritten */
  from: string;

  /** The From name; null when the message has none */
  fromName: string | null;

  /** The subject */
  subject: string;

  /** True when the From is not on a verified Sending domain: the message would not pass the Sending-domain gate at a provider that delivers outside */
  fromUnverified: boolean;

  /** When the message was captured */
  createdAt: Timestamp;
}

/** A Captured message as the list shows it: without the content */
model SiteCapturedMessageSummary {
  /** Unique identifier */
  id: EntityId;

  ...SiteCapturedMessageFields;
}

/** A message a Test inbox kept instead of sending it out: what the recipient would have received */
@resource("captured-messages")
@segment("captured-messages")
@parentResource(SiteWorkspaceResource)
model SiteCapturedMessageResource {
  /** Unique identifier */
  @key
  id: EntityId;

  ...SiteCapturedMessageFields;

  /** The HTML part */
  html: string;

  /** The text part */
  text: string;

  /** The header lines the message would carry on the wire, in order */
  headers: SiteCapturedMessageHeader[];

  /** The URLs of the links of the HTML part, tracker links as the recipient would get them */
  links: string[];
}

// Read and delete side of the Test inbox. Nested via @parentResource + @autoRoute.
@tag("Site Captured Messages")
namespace SiteCapturedMessages {
  /** List the Captured messages of the workspace, newest first, without their content (read one for it). Every member may read. */
  @listsResource(SiteCapturedMessageResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(
    ...ResourceCollectionParameters<SiteCapturedMessageResource>,
    ...PageQuery,
  ):
    | Page<SiteCapturedMessageSummary>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Read one Captured message in full: headers, HTML, text and links. Every member may read. */
  @readsResource(SiteCapturedMessageResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op get(...ResourceParameters<SiteCapturedMessageResource>):
    | SiteCapturedMessageResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Delete one Captured message. Every member may delete. */
  @deletesResource(SiteCapturedMessageResource)
  @delete
  @autoRoute
  @requiresPermissions(#[])
  op delete(...ResourceParameters<SiteCapturedMessageResource>):
    | NoContentResponse
    | BadRequestProblem
    | NotFoundProblem;

  /** Delete every Captured message of the workspace. Every member may delete. */
  @collectionAction(SiteCapturedMessageResource, "all")
  @delete
  @autoRoute
  @requiresPermissions(#[])
  op deleteAll(...ResourceCollectionParameters<SiteCapturedMessageResource>):
    | NoContentResponse
    | BadRequestProblem
    | NotFoundProblem;
}
