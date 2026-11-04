---
status: accepted
---

# On-site forms load from a host of their own, and the audience answer names only forms

Display rules split the work: the browser decides when, the server decides whom (ADR 0089). The
tracker today is one script, `t.js`, and `/collect`, both on the Workspace's Tracking domain, a CNAME
to the Instance's edge (ADR 0059, ADR 0071). uBlock Origin on Firefox and Brave resolve a CNAME and
apply their lists to the canonical name, so one listing of the edge's name would block every
customer at once. Popups annoy more than tracking does, and
an annoyance list that blocks them should not take clicks, attribution and Identify with them.
Klaviyo serves its forms from `static.klaviyo.com` and Sleeknote keeps its analytics on a host of
its own; Drip, Customer.io and Braze are blocked as one host. Decided in the delivery ticket
(getsphericon/sphericon#956).

## Decisions

- **Forms load from the Form host,** a hostname of the Instance's own apart from the Tracking
  domain: the display code, the Workspace's form config and the audience answer. A Self-hoster
  sets it, and it defaults to the Instance's host. A Form submit still goes to `/collect` on the
  Tracking domain. Blocking the Form host stops forms only. The customer's CSP lists one more host.
- **The snippet does not change.** `t.js` fetches the config and loads the display code only when a
  live form may show on the page.
- **The config is public and cached.** It holds the live versions of the Workspace's forms: content,
  triggers, page rules, schedule, frequency and the kind of audience. It holds no Segment, no
  Condition and no draft, since a Condition can reveal what a customer selects on ("lifetime value
  over 1000"). It is cached at the edge for about a minute, so a Publish shows within one.
- **The audience answer names only forms.** When a live form has an audience other than everyone,
  the tracker asks once per visit which forms its Visitor is admitted to; the server evaluates the
  copied Conditions (ADR 0089) for the Visitor's Contact at that moment and returns form ids, nothing
  else. The tracker asks again after an Identify or a Form submit. It counts against the existing
  collect limits per Workspace and per IP (ADR 0025).
- **No merge tags in a form.** A form that greeted a Contact by name could not be cached and would
  hand a Contact's attributes to whoever holds the public key and a Visitor id.
- **New contracts under ADR 0036:** the display code's path, the config and audience paths on the
  Form host, the `data-sphericon-placement` attribute, the URL parameter that shows a form, and the
  call that shows a form from the site's code.

## Considered options

- **Separate paths on the Tracking domain.** A path rule would block forms alone, but a listing of
  the edge's canonical name would still block everything.
- **A second DNS record at the customer** (`forms.example.com`). The same effect as the Form host,
  for one more setup step per customer.
- **One personal request per page view, uncached.** Every page view of every site would reach the
  Instance.
