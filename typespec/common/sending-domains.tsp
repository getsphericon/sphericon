// The Sending domain write bodies: /site and /api take the same input.

namespace Sphericon;

/** Add a Sending domain; mints the DKIM keypair and returns the DNS records to publish. */
model CreateSendingDomainInput {
  /** The domain to authenticate, e.g. "mail.acme.com" (normalized to lowercase ASCII) */
  domain: string;

  /** DKIM selector; defaults to "sphericon" when omitted */
  dkimSelector?: string;
}
