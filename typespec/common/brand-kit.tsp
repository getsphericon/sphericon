// The Workspace's Brand kit: /site and /api show the same.

namespace Sphericon;

/** The Workspace's Brand kit (ADR 0040): its colors, fonts, logo, website, button style and footer.
 * It sets the palette the editor offers and what an agent is told to use: write a message in these
 * values.
 * Its values are written into a message when the message is authored, so saving it never changes a
 * message already written. A Workspace that has not saved one has the defaults and `saved` false. */
model BrandKitResource {
  /** False until the Brand kit is first saved; the values are then the defaults */
  saved: boolean;

  /** Brand color of buttons and links, `#rrggbb` */
  accentColor: string;

  /** Color of text, `#rrggbb` */
  textColor: string;

  /** Color of the message background, `#rrggbb` */
  backgroundColor: string;

  /** Font of headings: a CSS font-family list of web-safe families */
  @untrusted
  headingFont: string;

  /** Font of body text: a CSS font-family list of web-safe families */
  @untrusted
  bodyFont: string;

  /** Address (http or https) of the logo image; empty for none */
  @untrusted
  logoUrl: string;

  /** Address (https) of the business's website, where the starter Templates' buttons link; empty for none. Link a message there rather than to a placeholder */
  @untrusted
  websiteUrl: string;

  /** Color of the text on a button, `#rrggbb` */
  buttonTextColor: string;

  /** Corner radius of a button in pixels, 0 to 32 */
  buttonRadius: int32;

  /** Plain text printed under the starter messages, at most 500 characters; empty for none */
  @untrusted
  footerText: string;

  /** When the Brand kit was last saved; absent while it is not saved */
  updatedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): a save names it. A Brand kit not saved yet is revision 1 */
  revision: Revision;
}
