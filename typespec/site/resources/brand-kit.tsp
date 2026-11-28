using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Save the Brand kit: the first save creates it and the starter Templates, a later one replaces
 * its values. */
model SiteSaveBrandKitInput {
  /** The Brand kit's `revision` the save was made on (ADR 0119): the one it was read with, 1 for the first save */
  revision: Revision;

  /** Brand color of buttons and links, `#rrggbb` */
  accentColor: string;

  /** Color of text, `#rrggbb` */
  textColor: string;

  /** Color of the message background, `#rrggbb` */
  backgroundColor: string;

  /** Font of headings: letters, digits, spaces, commas and hyphens only */
  headingFont: string;

  /** Font of body text: letters, digits, spaces, commas and hyphens only */
  bodyFont: string;

  /** Address (http or https) of the logo image; empty for none */
  @maxLength(2048)
  logoUrl: string;

  /** Absolute https address of the business's website, where the starter Templates' buttons link; empty for none */
  @maxLength(2048)
  websiteUrl: string;

  /** Color of the text on a button, `#rrggbb` */
  buttonTextColor: string;

  /** Corner radius of a button in pixels, 0 to 32 */
  @minValue(0)
  @maxValue(32)
  buttonRadius: int32;

  /** Plain text printed under the starter messages, at most 500 characters; empty for none */
  @maxLength(500)
  footerText: string;
}

@route("/workspaces/{workspaceId}/brand-kit")
@tag("Site Brand Kit")
namespace SiteBrandKit {
  /** Read the Brand kit. Every member may read it. */
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId): BrandKitResource | Problem;

  /** Save the Brand kit (owner and admin only). The first save also creates the starter
   * Templates (a welcome, a receipt, a digest and a one-button message) in its brand; a later
   * save creates none and changes no message already written. A change is recorded as an Audit
   * entry. */
  @put
  @requiresPermissions(#[Permission.brandKitManage])
  op save(@path workspaceId: WorkspaceId, @body body: SiteSaveBrandKitInput):
    | BrandKitResource
    | Problem;

  /** Create the starter Templates again from the saved Brand kit (owner and admin only). It adds
   * new Templates and never touches one that exists or a message already written. Answers 422
   * until the Brand kit has been saved. */
  @post
  @route("/starter-templates")
  @requiresPermissions(#[Permission.brandKitManage])
  op recreateStarterTemplates(@path workspaceId: WorkspaceId):
    | {
        @statusCode statusCode: 201;
        @body body: SiteEmailTemplateResource[];
      }
    | Problem;
}
