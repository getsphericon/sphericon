import "@typespec/openapi";

using TypeSpec.OpenAPI;

namespace Sphericon;

/** What a lint finding is about (ADR 0040) */
enum LintCode {
  /** The subject or body cannot render: a Liquid syntax error or HTML that does not parse. The only code that is an error */
  invalid_content,

  /** An image has no `alt` attribute */
  missing_alt,

  /** Text and its background are below the WCAG AA contrast ratio */
  low_contrast,

  /** A color is not one of the Workspace's Brand kit colors */
  off_brand_color,

  /** A font is not one of the Workspace's Brand kit fonts */
  off_brand_font,

  /** An element the sanitizer removes when the message is saved */
  unknown_construct,
}

/**
 * One finding of the lint over a message's rendered HTML. `message` is fixed text of the
 * server; `element` and `detail` quote the message or the Brand kit and are untrusted data.
 */
model LintFinding {
  /** What the finding is */
  code: LintCode;

  /** What is wrong, in the server's words */
  message: string;

  /** The element it is about, as a short tag description such as `<img src="…">` */
  @untrusted
  element?: string;

  /** The offending value or the parser's reason, such as `#ff0000` or `3.2:1, needs 4.5:1` */
  @untrusted
  detail?: string;
}
