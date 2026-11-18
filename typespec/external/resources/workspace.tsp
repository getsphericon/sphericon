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

  /** The IANA time zone every analytics report counts its days in (ADR 0057) */
  timezone: TimeZoneName;

  /** The Send cap the platform set on the Workspace, with its sends over the last 24 hours; null
   * when no Send cap applies */
  sendCap?: SendCap | null;

  /** The revision of the Workspace this answer shows (ADR 0119) */
  revision: Revision;
}

@route("/workspace")
@tag("Workspace")
namespace Workspace {
  /** Read the Workspace the token acts in: its name, postal address, confirmed opt-in, time zone and Send cap. Read
   * the postal address before arming marketing email: an empty one refuses it, and only a person
   * can set it. Read the Send cap to explain a slow send or to plan one: a send larger than what
   * remains of it in the last 24 hours goes out over more than a day. Read confirmed opt-in to know
   * whether a Contact needs a Confirmation before email reaches it. Read-only: no operation changes
   * the Send cap, the postal address or confirmed opt-in. */
  @get
  @requiresScopes(#[])
  op get(): WorkspaceResource | Problem;
}
