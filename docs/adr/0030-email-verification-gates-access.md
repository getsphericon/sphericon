# Email verification gates access

A registration takes a name and an email only and creates an Unverified User that cannot log in,
holds no password and owns no Workspace, until its email is proven by a link sent to it; that
link is where the password is set. No password therefore exists before the address is proven,
for sign-up, Invitation and password reset alike.
An Invitation to an address with a Verified User needs that User's own session (its email equal
to the Invitation's). An Invitation to an address with an Unverified User is treated like one to
a new address: the link proves the address, the invitee sets name and password, and the earlier
password is discarded (this wins over keeping it). This closes account pre-hijacking: an attacker who
registers a victim's address first (and knows its password) can no longer be attached to the
Workspaces the victim is later invited to.

`SPHERICON_REQUIRE_EMAIL_VERIFICATION` (default `true`) turns the gate off for a self-hosted install
without outbound mail, so registration yields a verified User at once; the SaaS runs it on. It is
an explicit self-hoster choice, never inferred from whether SMTP is configured, because a mail
misconfiguration must not silently disable the protection.

## Considered Options

- **Soft verification** (log in at once, verify within N days, as GitLab's "Soft" mode): leaves
  the window the attack uses; rejected.
- **Reset the password only inside `AcceptInvitation`**: closes the invitation path but leaves
  the unverified account able to log in and act, and every future path that attaches a
  Membership or Workspace to a User would need the same care; kept only as defence in depth.
- **Gate off whenever SMTP is unset**: breaks silently on a misconfigured mail server; rejected
  for the explicit flag.

## Consequences

- Registration creates the first Workspace and sends the welcome email when the address is
  verified, not at sign-up.
- Registering and the invite lookup's `hasAccount` used to reveal whether an address is taken;
  ADR 0031 closes that.
- Unverified Users hold their address until pruned: a daily job deletes those with no Membership
  older than `SPHERICON_UNVERIFIED_USER_RETENTION_DAYS` (default 7, `0` disables). A pending Invitation to
  a pruned address is untouched; accepting it follows the new-address path.
