using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The Workspace's Brand kit (ADR 0040): its colors, fonts, logo, button style and footer. It sets
 * the palette the editor offers and what an agent is told to use. Its values are written into a
 * message when the message is authored, so saving it never changes a message already written. A
 * Workspace that has not saved one has the defaults and `saved` false. */
model SiteBrandKitResource {
  /** False until the Brand kit is first saved; the values are then the defaults */
  saved: boolean;

  /** Brand color of buttons and links, `#rrggbb` */
  accentColor: string;

  /** Color of text, `#rrggbb` */
  textColor: string;

  /** Color of the message background, `#rrggbb` */
  backgroundColor: string;

  /** Font of headings: a CSS font-family list of web-safe families */
  @extension("x-untrusted", true)
  headingFont: string;

  /** Font of body text: a CSS font-family list of web-safe families */
  @extension("x-untrusted", true)
  bodyFont: string;

  /** Address (http or https) of the logo image; empty for none */
  @extension("x-untrusted", true)
  logoUrl: string;

  /** Color of the text on a button, `#rrggbb` */
  buttonTextColor: string;

  /** Corner radius of a button in pixels, 0 to 32 */
  buttonRadius: int32;

  /** Plain text printed under the starter messages, at most 500 characters; empty for none */
  @extension("x-untrusted", true)
  footerText: string;

  /** When the Brand kit was last saved; absent while it is not saved */
  updatedAt?: Timestamp | null;
}

/** Save the Brand kit: the first save creates it and the starter Templates, a later one replaces
 * its values. */
model SiteSaveBrandKitInput {
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
  logoUrl: string;

  /** Color of the text on a button, `#rrggbb` */
  buttonTextColor: string;

  /** Corner radius of a button in pixels, 0 to 32 */
  buttonRadius: int32;

  /** Plain text printed under the starter messages, at most 500 characters; empty for none */
  footerText: string;
}

@route("/workspaces/{workspaceId}/brand-kit")
@tag("Site Brand Kit")
namespace SiteBrandKit {
  /** Read the Brand kit. Every member may read it. */
  @get
  op get(@path workspaceId: WorkspaceId):
    | SiteBrandKitResource
    | NotFoundProblem;

  /** Save the Brand kit (owner only). The first save also creates the starter
   * Templates (a welcome, a receipt, a digest and a one-button message) in its brand; a later
   * save creates none and changes no message already written. A change is recorded as an Audit
   * entry. */
  @put
  op save(@path workspaceId: WorkspaceId, @body body: SiteSaveBrandKitInput):
    | SiteBrandKitResource
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Create the starter Templates again from the saved Brand kit (owner only). It adds
   * new Templates and never touches one that exists or a message already written. Answers 422
   * until the Brand kit has been saved. */
  @post
  @route("/starter-templates")
  op recreateStarterTemplates(@path workspaceId: WorkspaceId):
    | {
        @statusCode statusCode: 201;
        @body body: SiteEmailTemplateResource[];
      }
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
