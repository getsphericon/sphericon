---
status: accepted
---

# An Instance runs in one Mode, and the Mode, not a flag, decides what differs

The same binary is run as a self-hosted Instance and as the SaaS, and the two had drifted apart as a
set of independent switches (sign-up, its survey, email verification, Telemetry, the Release
check), each of which a deployment had to set the same way every time. A **Mode**, `selfhosted` or
`saas`, now names the deployment, and everything that differs between the two follows from it;
the switches are gone. `SPHERICON_MODE` selects it at start, empty means `selfhosted`, and any
other value stops the start.

## Decisions

- **The Mode is not a license.** The Enterprise features stay behind the License key
  ([[0014-open-core-boundary]]); `saas` unlocks none of them, so setting it on a self-hosted
  Instance gains nothing. The two are independent, and `saas` refuses to start without a License
  key that has `staff`.
- **One binary, read at start.** No build tag and no second image: the Mode is configuration, like
  the License key.
- **`saas`** keeps sign-up open, asks the Sign-up survey, requires email verification, sends no
  Telemetry report and makes no Release check. It also needs the values only a deployment can give:
  the Staff member secret, the internal listener address, the Tracking CNAME target and a system
  email provider. A missing one stops the start and names the variable.
- **`selfhosted`** leaves sign-up open only until the Instance has its first Owner, then closes it:
  the others come through an Invitation, and `sphericon user create-owner` stays
  ([[0042-sign-up-can-be-closed-and-the-first-owner-is-created-deliberately]]). Email
  verification is not required there, since the only sign-up is the first Owner's and an
  Invitation link already proves its address ([[0030-email-verification-gates-access]]). There is
  no Sign-up survey. The Release check is on. The Telemetry report is the Owner's choice in the app,
  and `DO_NOT_TRACK=1` still switches it off ([[0032-instance-telemetry-report-and-release-check]]).
- **The Staff surface needs both** the `saas` Mode and the `staff` feature of the License key
  ([[0026-operator-console-and-abuse-detector]]). Audit log, Export streams and advanced retention
  depend on the key alone.
- **The Mode is not in the API or the UI of a production Instance.** The client already reads
  capabilities (a locked Telemetry setting, `signup_closed`, a 404 on the Staff routes), and the word
  `saas` stays out of the open app's contracts. A non-production server alone also states its Mode in
  the Instance state, and the app shows it in a badge, so a developer sees which one they run (as
  only a development server sends the cause of a 500). Self-hosters' own metrics (Prometheus,
  VictoriaMetrics, OpenTelemetry) do not depend on the Mode.
- **A development or end-to-end Instance trusts a development license key**, one signed by a key
  pair committed to the repository, so `mise run dev:saas` and the end-to-end suite can start `saas`.
  It does so only outside a release build (`Config.TrustsDevLicense`: a development or end-to-end
  env and not `buildinfo.IsRelease`): an empty `SPHERICON_ENV` counts as development, so the env
  alone would let a self-hoster who never set it unlock `ee/`.

## Considered Options

- **Keep the switches and add a Mode that sets their defaults**: an explicit variable would still
  win, so a deployment could be half one and half the other, and the switches stay to be documented
  and tested; rejected.
- **A build-time edition**: a second artifact to ship and test, against "one binary for everyone"
  ([[0014-open-core-boundary]]); rejected.
- **A Mode that grants the Enterprise features**: it is the "one entitlement" ADR 0014 rejects, and
  it would hand every self-hoster the `ee/` features for one variable; rejected.
- **Keep sign-up open by default in `selfhosted`** (the old default): the first person to reach a
  fresh public Instance gets its first Workspace, with no switch left to close it; rejected for
  closing it after the first Owner.

## Consequences

- ADRs 0014, 0026, 0030, 0032 and 0042 are amended in place to point here; the switches they
  introduced (`SPHERICON_SIGNUP_OPEN`, `SPHERICON_SIGNUP_SURVEY`,
  `SPHERICON_REQUIRE_EMAIL_VERIFICATION`, `SPHERICON_TELEMETRY`, `SPHERICON_RELEASE_CHECK`) are
  removed from the configuration, with no fallback (the project is greenfield).
- The test harness defaults to `saas`, since its test License carries every feature and the existing
  tests exercise the gates the SaaS has on (verification, the Sign-up survey, open sign-up); a test
  of `selfhosted` sets the Mode (`testhelper.WithMode`). The end-to-end suite runs `saas` too, with
  the development License key.
- ADR 0032 rejected "a this-is-the-SaaS flag" because it would only turn the report off. The Mode
  now decides much more than that, which is why it is accepted here.
