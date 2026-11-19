// The signing secret of an HTTP call (an Automation's HTTP step, a Rule's http Action): one scalar,
// so its format is written once.

namespace Sphericon;

/** A Standard Webhooks signing secret: a non-empty base64 key, usually prefixed `whsec_`, such as
 * `whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw`. */
@pattern("^(?:whsec_)?(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$")
scalar SigningSecret extends string;
