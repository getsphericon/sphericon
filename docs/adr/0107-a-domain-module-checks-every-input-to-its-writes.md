---
status: accepted
---

# A domain module checks every input to its writes

> Refines [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]: it gave a
> domain module "the rules of its own rows"; this says that no caller holds one of them for it.

## Context

Only the owner writes a table (ADR 0102, `entrywrite`). Yet several exported writes trust their
callers to have checked what they are given, so a rule of the rows holds only on the paths whose
caller remembered it:

- **The caller holds the owner's rule.**
  - `contacts.ApplyTagIn` trims and refuses a blank Tag name with `tags.ErrInvalidName`, and
    `tags.EnsureIn` stores whatever it gets.
  - The Enterprise Staff console checks a Hold's expiry and action URL, and `holds.Set` checks
    nothing.
  - `users.CreateUnverified` checks the name, while `users.CreateVerifiedIn` says "the caller has
    checked the name" and relies on the `signup` use case.
  - `oauthclients.Register` leaves the redirect URIs to the OAuth handler.
  - `unsubscribes.Add` leaves the address's form to its callers.
- **The contract holds the bound, and only HTTP sees it.** The validation package says ogen "has
  already applied the contract's constraints", and `check:copies` refuses a Go constant equal to a
  contract bound. MCP goes through the same ogen handlers, but the CLI, the jobs, the use cases and
  the other modules do not, and they skip the bounds:
  - a Hold's reason code and message;
  - the Send cap, which `suspension.SetSendCap` and `instancestates.SetDefaultSendCap` leave to the
    contract;
  - the attribution window and the Audit log retention of a Workspace;
  - the five headers of an http Action;
  - the form of a Contact's email.
- **Names are stored as they come.** Segments, Rules, Automations, Templates and Broadcasts store an
  untrimmed name, and only ent's `NotEmpty` stands behind it.
- **A schema validator answers the client.** `problem` turns an `ent.ValidationError` into a 422 whose
  detail is ent's English text, such as `ent: validator failed for field "Segment.name"`. Only
  `emailtemplates` replaces it with a refusal of its own.
- **Five forms of refusal:**
  - the `validation` constructors;
  - `&validation.Error{}` literals (35, 25 of them in `integrations`);
  - `validation.Form` struct tags, which restate contract bounds;
  - `orders`' own `RefusalError` type;
  - a bare `errors.New` that a handler maps by hand.
- **The Email case is kept four ways:**
  - `emailcase.Hook` with CHECK constraints for Users, Invitations and Contacts;
  - `throttle.NormalizeEmail` for Staff members and Invitations;
  - `suppressions.NormalizeDestination`;
  - a private copy each in `contacts` and in the `contacterasure` use case.
- **Domain policy in the job runner:**
  - `internal/jobs/rules.go` re-checks that an Automation is active before enrolling, which
    `automations` already does under its lock. It also lists which errors end a Rule Action as Failed.
  - `internal/jobs/webhook.go` skips a disabled endpoint, opens its secret and filters events by the
    endpoint's subscription.
  - `internal/jobs/send_quota.go` chooses which Integrations get a quota refresh.
- **The CLI repeats checks** its modules make.
- **Hooks are installed in several places.** The Enterprise Edition's Usage hook is installed apart
  from `entclient`, in three places, and has no guard for a client outside a transaction, unlike the
  core's hooks.

## Decisions

- **Every exported write of a domain module normalises and checks all of its input itself.** No doc
  comment says "the caller has checked". A caller passes the raw value and gets the module's refusal.
- **The contract states a bound; the owner applies it.**
  - TypeSpec stays the one place a value's bound is written.
  - The bounds are generated into Go constants, the way the scope vocabulary is generated into the
    kernel (ADR 0102).
  - The module that owns the field checks each write against them. A Go copy of a contract bound
    stops existing, and so does the `contract-copy` marker.
  - On HTTP and MCP a value is checked twice, by ogen first, then by the module. Every other caller
    meets only the module's check.
- **One rule for a name.**
  - One TypeSpec scalar states a name's length.
  - Its generated rule in the kernel trims the name, refuses a blank one and applies the length.
  - Every owner of a named row (Tag, Segment, Rule, Automation, Template, Broadcast, Webhook endpoint,
    and the others the contract names) calls it on create and update.
- **One function keeps the Email case.**
  - A kernel function trims and lower-cases an address.
  - Every owner of an address column calls it on write and on every lookup by address, so `throttle`,
    `suppressions`, `contacts` and `contacterasure` lose their copies.
  - Every column that stores an address has the lower-case CHECK, so a write that skips the function
    fails.
  - `emailcase.Hook` is removed: with one writer per table, the hook only guarded the owner against
    itself, which the CHECK does loudly.
- **A schema validator is a backstop.** ent's validators (`NotEmpty`, `MaxLen`) stay. One that fires
  means the module missed a check, so it is a fault (500), like a CHECK (ADR 0106). The
  `ent.ValidationError` case leaves `problem`.
- **One form of refusal: the `validation` constructors.**
  - These are `Refuse`, `RefuseField`, the `Conflict` ones and `Fields` for several fields at once.
  - The `validation.Error` literal, the `validation.Form` tags, `orders`' type and refusals made with
    `errors.New` are removed.
- **The job runner holds no domain rule.**
  - Any `validation` refusal ends a job as its final outcome, and any other error retries it. This one
    rule replaces each job's list of errors that end it.
  - The webhook delivery's checks and event filter move into `webhookendpoints`.
  - The quota refresh's choice moves into `integrations`.
  - The enroll Action's pre-check is removed.
  - The CLI's repeated checks are removed.
- **A hook is kept only where a write must move another module's log in its own transaction**, such as
  the Workspace and User feeds, the Tracking host log and the Usage peak.
  - `entclient` installs every one of them, the Enterprise Edition's included, which it takes as a
    parameter.
  - Each hook has a guard that refuses the change on a client outside a transaction.
- **Two checks hold the rule:**
  - An analyzer refuses a reference to another package's refusal sentinel outside `errors.Is`, so a
    module cannot refuse on another's behalf with the owner's own words.
  - A test refuses a generated bound that no non-test code of its field's owner uses. The owner comes
    from the ownership map of `entrywrite`. A bound with no column, such as the number of an http
    Action's headers, names its owner explicitly.

## Considered Options

- **Only ogen checks, as before.** Every path that is not HTTP stays unchecked, and each new driver
  (a job, a CLI command, a use case) would have to restate the contract.
- **Value rules live in the modules, and the contract omits them.** The contract would stop telling a
  client the bounds, against the contract-first pipeline (ADR 0016, ADR 0069).
- **Keep the Email case hook and add the missing columns to it.** With one writer per table, a hook
  duplicates the owner's write function. It also lives outside the module, in the wiring's list.
- **Struct-tag validation for every module.** The tags restate the contract's bounds by hand, which is
  what generation removes.

## Consequences

- Every owner whose writes trust a caller, or leave a bound to the contract, gains the checks.
  Each caller check that duplicates one is removed: the deletion test finds each by removing it.
- `internal/kernel/validation`'s package comment, the `check:copies` section of `CODING_STANDARDS.md`
  and review rule 5 ("a driver checks no rule of a row") change with the code.
- The Email case entry of the glossary already describes the rule. It is the code that changes.
