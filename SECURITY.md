# Security Policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub private vulnerability reporting:
open the [Security tab](https://github.com/getsphericon/sphericon/security) of this repository and
choose **Report a vulnerability**. This is the only supported channel; there is no security
mailbox.

Do not open a public issue, pull request or discussion for a suspected vulnerability.

Please include:

- the affected version or commit and how you run sphericon (self-hosted, Helm, source);
- a description of the impact and the steps to reproduce it;
- a proof of concept, if you have one.

## Supported versions

Security fixes are released for the latest minor release of the current major version
(currently 1.x). Older releases are not patched; upgrade to the latest release. Unreleased
`main` is fixed on a best-effort basis.

## Scope

In scope:

- the sphericon server and its three API surfaces (`/site`, `/api`, `/collect`);
- the web application and the tracker snippet (`/t.js`, `@sphericon/analytics`);
- authentication, authorization and workspace isolation (cross-tenant access);
- the Enterprise code under `ee/`;
- the Helm chart and the deployment examples in this repository.

Out of scope:

- vulnerabilities in third-party dependencies with no demonstrable impact on sphericon
  (report those upstream);
- findings that need a compromised self-hoster, database or host, or physical access;
- denial of service through volumetric traffic, and missing rate limits on their own;
- social engineering, phishing, and attacks on the maintainers or on users' infrastructure;
- misconfiguration of a deployment that contradicts the documented hardening guidance;
- automated scanner output without a demonstrated exploit.

## What to expect

This is a best-effort policy from a small team. Targets, in calendar days:

| Step                               | Target                                    |
| ---------------------------------- | ----------------------------------------- |
| Acknowledge your report            | 3 days                                    |
| Initial assessment (valid or not)  | 7 days                                    |
| Fix or mitigation for valid issues | 90 days at most, sooner for critical ones |

We will keep you updated in the private advisory, publish the fix and a GitHub security
advisory (with a CVE when appropriate), and credit you unless you prefer to stay anonymous.
Please keep the details private until the advisory is published, or until 90 days have
passed without a fix.

## Safe harbor

We consider good-faith security research under this policy to be authorized. We will not
pursue or support legal action against you for research that stays in scope, avoids privacy
violations, data destruction and service disruption, accesses only the data needed to
demonstrate the issue, and follows the disclosure process above. Test against your own
instance, never against other people's workspaces or data.

## Maintainer checklist

- [ ] Private vulnerability reporting is enabled in the repository under **Settings > Code
      security** (**Private vulnerability reporting**). The reporting channel above does not work
      until a maintainer turns it on; it is a manual setting and is never changed by automation.
