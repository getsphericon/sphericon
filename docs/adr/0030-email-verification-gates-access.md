# Email verification gates access

> **Amended by [[0060-an-instance-runs-in-one-mode]]:** `SPHERICON_REQUIRE_EMAIL_VERIFICATION` is gone.
> The gate is on in `saas` and not required in `selfhosted`, where the only sign-up is the first
> Owner's and an Invitation link proves its address. The text below is the decision as first taken.

A Sign-up takes a name, an email and a password. It creates an Unverified User that cannot
log in and owns no Workspace until its email is proven by a link sent to it. The password given
at Sign-up is trusted only when the link is opened in the browser that signed up (the
Sign-up browser): that browser holds an httpOnly cookie with a random value, and the link's
signed token carries its hash. A match keeps the password, starts the session and lands the User
in its first Workspace. Any other browser (the mail opened on a phone, a forwarded link, an
attacker who signed up with someone else's address) gets a page that sets a new password, and the
password from the form is discarded; setting it starts the session too. So a password set before
the address is proven never becomes live unless the one who proved the address is the one who set
it. The same holds for Invitation and password reset: their link is where the password is set.
An Invitation to an address with a Verified User needs that User's own session (its email equal
to the Invitation's). An Invitation to an address with an Unverified User is treated like one to
a new address: the link proves the address, the invitee sets name and password, and the earlier
password is discarded (this wins over keeping it). This closes account pre-hijacking: an attacker who
signs up with a victim's address first (and knows its password) can no longer be attached to the
Workspaces the victim is later invited to.

`SPHERICON_REQUIRE_EMAIL_VERIFICATION` (default `true`) turns the gate off for a self-hosted install
without outbound mail, so a Sign-up yields a verified User at once; the SaaS runs it on. It is
an explicit self-hoster choice, never inferred from whether SMTP is configured, because a mail
misconfiguration must not silently disable the protection.

A repeated Sign-up of an Unverified address replaces its password and Sign-up browser;
earlier mails stop working (their token is bound to the earlier hash). A Sign-up of a Verified
address stores nothing but still hashes the password, so the answer's timing does not say the
address is taken (ADR 0031).

The Sign-up browser cookie's Path is the `/site/auth` prefix, shared by verify and resend, so both
read it. Verify cannot clear it (the answer has one `Set-Cookie`, the session's), so it stays until
its lifetime, the link's, ends. Resend binds the new link to whatever Sign-up browser cookie the
caller holds, none when it holds none. The cookie is `SameSite=Strict`: the browser does not send it
on the cross-site click from the mail, but sends it on the SPA's later same-site verify request,
which is the one that is compared. Two Sign-ups of one new address can race, and the loser
then replaces the winner's password, as a repeated Sign-up does, and the winner's link
stops working.

## Considered Options

- **Password only on the link** (the first version of this ADR): closes pre-hijacking outright,
  but asks a person who just filled in a form to choose a password again and then sign in; rejected
  as too harsh.
- **Password in the form, kept whoever opens the link**: an attacker who signs up with a victim's
  address with a password of their own owns the account the moment the victim follows the mail;
  rejected, the Sign-up browser check is what keeps it safe.

- **Soft verification** (log in at once, verify within N days, as GitLab's "Soft" mode): leaves
  the window the attack uses; rejected.
- **Reset the password only inside `AcceptInvitation`**: closes the invitation path but leaves
  the unverified account able to log in and act, and every future path that attaches a
  Membership or Workspace to a User would need the same care; kept only as defence in depth.
- **Gate off whenever SMTP is unset**: breaks silently on a misconfigured mail server; rejected
  for the explicit flag.

## Consequences

- A Sign-up gets its first Workspace and the welcome email when the address is
  verified, not when it is submitted. Verifying signs the User in, from either page; no Unverified User
  or Second factor stands in the way, as a new User has none.
- Signing up and the invite lookup's `hasAccount` used to reveal whether an address is taken;
  ADR 0031 closes that.
- Unverified Users hold their address until pruned: a daily job deletes those with no Membership
  older than `SPHERICON_UNVERIFIED_USER_RETENTION_DAYS` (default 7, `0` disables). A pending Invitation to
  a pruned address is untouched; accepting it follows the new-address path.
