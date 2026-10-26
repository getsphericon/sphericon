# Sending domains: sphericon-native DKIM, verified-domain required to send

To reach deliverability parity, mail must be authenticated (DKIM/SPF/DMARC) and sent from a
domain the workspace controls — today `from_email` is a free string, nothing is signed, and
nothing verifies the From domain. We add a first-class, workspace-scoped **Sending domain**
entity and gate sending on it.

**sphericon signs outbound mail itself (native DKIM)** rather than delegating to the provider:
sphericon generates a per-domain keypair, signs every message in the send path, and the user
publishes one `selector._domainkey` TXT. This makes the sending identity **independent of the
Integration** — the same verified domain works identically across SMTP, SES, and any future
provider, with no re-verification when transport changes. Keys are stored encrypted (reuse the
existing Tink keyset); signing leans on a maintained Go DKIM library, not hand-rolled.

**A verified Sending domain is required to send.** `from_email` becomes a verified-domain +
local part; the send path rejects (RFC 7807) any From whose domain isn't a verified Sending
domain, on all three surfaces (Broadcast, Automation, Transactional). This is a breaking change
from the free-string `from_email` and needs a migration + an onboarding step.

## Considered options

- **Provider-delegated verification (SES manages DKIM)** (rejected): less code, but per-provider,
  breaks for raw SMTP, and couples the domain to a specific Integration — fragmenting the model
  the moment a second provider lands. Native signing keeps one transport-independent story.
- **Soft enforcement (send unsigned with a warning)** (rejected): unsigned mail from arbitrary
  domains _is_ the deliverability hole; it defeats the feature.

## Consequences

- **The gate is DKIM only.** Verification = the DKIM TXT is published and matches our key. SPF and
  DMARC are generated, shown, and status-checked but do **not** block: SPF depends on the
  transport's sending IPs (which native signing deliberately abstracts), and aligned DKIM already
  yields a DMARC pass. Gate on the one thing we control end-to-end.
- **Verified is a live property, kept as moments, not a flag.** A background re-check re-validates
  the DKIM DNS; if the record disappears the domain records `verification_lost_at` (its
  `verified_at` stays) and its sends block, with the owner notified the instant it is lost (mirrors the Workspace-suspension notify pattern). This
  prevents signing+sending for a domain whose DKIM receivers can no longer validate — silent
  deliverability rot — at the cost that a DNS edit can halt a live workspace (mitigated by
  immediate notification and trivial re-verification).
- Distinct from **Sending source** (the unsubscribe scope) despite the similar name — one is an
  authenticated identity, the other a consent boundary.
- **Still open after the enforcement slice.** The send gate and the verified→unverified owner
  notification are built, but one consequence the decision names is not (the Broadcast `from_email`
  is no longer a free string, see the amendment on the Broadcast From below):
  **local dev sending** now requires a verified domain, and the periodic re-check runs against
  real DNS, so a seeded dev domain re-verts unless its DKIM TXT is actually published — the
  Mailpit send loop needs a dev escape (a dev `LookupTXT` that trusts seeded domains, or a
  dev-only gate bypass) or it stays blocked.

## Amendment: the gate lives in Outbound send and holds, it does not fail

The verified-Sending-domain gate runs once, inside the Outbound send module
([[0015-outbound-send-single-chokepoint]]), reading the stored `verified` value maintained by the
background re-check (no live DNS lookup per message). An unverified domain is a source-level
**`Held`** outcome: a Broadcast pauses and resumes when the domain is verified again, instead of
failing every remaining recipient permanently because a DKIM record blipped. The plan-time
duplicate of the check is removed; planning asks the same module.

## Amendment: a Broadcast's From is checked when it is saved

A Broadcast's From email is a verified Sending domain plus a local part; there is no per-address
entity and the domain match is exact. Setting or changing it to an address on any other domain is a
422 on every surface (site, external API, MCP), by the same `messaging.HasVerifiedSendingDomain`
check the send gate uses, so the two cannot drift. The check is independent of the Integration
(identity is independent of it): a Workspace with only a Test inbox and no Sending domain leaves the
From empty, which keeps meaning the Integration's default. A write that leaves the From unchanged is
not re-checked, so a Broadcast held for a domain that lost verification after the save stays
editable; the send-time hold remains for that case. The form builds the From from a local part and a
Select of the verified Sending domains.
