using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The Workspace the token acts in */
model WorkspaceResource {
  /** The Workspace id: its short opaque public address */
  id: WorkspaceId;

  /** Display name */
  @untrusted
  name: string;

  /** The physical postal address printed in the footer of every marketing email (ADR 0156); empty
   * while the Workspace has none, and then Schedule of an email Broadcast and Publish of an
   * Automation with an email send step are a 409 postal_address_required. Read-only: only a person
   * sets it, in the console's Workspace settings. */
  @untrusted
  postalAddress: string;

  /** Whether email reaches an address only once it holds a Confirmation (confirmed opt-in, ADR
   * 0013); off, email is single opt-in by the Workspace's choice. Read-only: only a person turns it
   * on, in the console's Workspace settings, affirming that the current mailable list consented. */
  requireConfirmedOptIn: boolean;

  /** The Engagement tracking default (ADR 0120): whether a Destination with no Engagement tracking
   * consent recorded has its opens and clicks tracked. Read-only: only a person changes it, in the
   * console's Workspace settings. */
  engagementTrackingDefault: EngagementTrackingDecision;

  /** Whether the click of a tracked link adds a Link identity token to a destination on one of
   * `linkIdentityHosts` (ADR 0160), so the tracker there binds the anonymous visitor to the
   * recipient's contact. Read-only: only a person changes it, in the console's tracking settings. */
  linkIdentityEnabled: boolean;

  /** The hostnames of the Workspace's own sites a Link identity token may be added to a link of */
  linkIdentityHosts: string[];

  /** The IANA time zone every analytics report counts its days in (ADR 0057) */
  timezone: TimeZoneName;

  /** The Send cap the platform set on the Workspace, with its sends over the last 24 hours; null
   * when no Send cap applies */
  sendCap?: SendCap | null;

  /** The Workspace's sending rules (ADR 0134, ADR 0137): its Frequency caps and its Sending windows.
   * Read-only: only a person changes them, in the console's Workspace settings. */
  sendingRules: SendingRules;

  /** The revision of the Workspace this answer shows (ADR 0119) */
  revision: Revision;
}

@route("/workspace")
@tag("Workspace")
namespace Workspace {
  /** Read the Workspace the token acts in: its name, postal address, confirmed opt-in, Engagement tracking default, Link identity token settings, time zone, Send cap and sending rules. Read
   * the postal address before arming marketing email: an empty one refuses it, and only a person
   * can set it. Read the Send cap to explain a slow send or to plan one: a send larger than what
   * remains of it in the last 24 hours goes out over more than a day. Read confirmed opt-in to know
   * whether a Contact needs a Confirmation before email reaches it. Read the Engagement tracking
   * default to know whether a Destination with no consent recorded is tracked. Read the sending
   * rules to explain a marketing message that waits or is skipped: a Frequency cap limits the
   * marketing messages one Destination receives per period, and a Sending window the hours a
   * marketing text goes out in the recipient's local time. Read-only: no operation changes the Send
   * cap, the postal address, confirmed opt-in, the Engagement tracking default or the sending
   * rules. */
  @get
  @requiresScopes(#[])
  op get(): WorkspaceResource | Problem;
}
