# Enforcing the contribution flow on GitHub (rulesets, CODEOWNERS, fork-safe CI, agent PRs)

Researched 2026-10-23 against primary sources only: docs.github.com (including the REST API
reference), the docs of the tools cited (zizmor, Claude Code, Codex, Cursor,
amannn/action-semantic-pull-request) and read-only `gh api` GET calls against
`getsphericon/sphericon`. Every factual claim carries its source URL. Anything not confirmed on a
primary page fetched during this research is marked **unverified**. Repo facts come from reading
this repo (`.github/`, `cmd/upgradecheck`, `CODING_STANDARDS.md`, `mise/tasks.toml`) and from the
live GitHub state noted in section 0.

**Short answer:** the platform can enforce _which checks must pass, who must review which paths,
the merge method, and who may bypass_. It cannot enforce that a contributor (or their agent) read
anything, ran anything locally, or wrote a truthful PR body. So every rule in `AGENTS.md` that
matters must be (a) a CI job, (b) a required check in a ruleset that the PR cannot rewrite, and
(c) for judgement calls (marker comments, migrations), a CODEOWNERS-gated human review. Today the
repo's only required check is `license/cla`, and the admin role bypasses everything, so almost none
of the flow is platform-enforced yet (section 0).

---

## 0. Current state of getsphericon/sphericon (read with `gh api`, GET only)

- Public repo, owned by an Organization on the **free** plan
  (`gh api orgs/getsphericon` -> `type: Organization`, `plan: free`).
- One ruleset, "General", targeting `~DEFAULT_BRANCH`, `enforcement: active`, rules:
  `deletion`, `non_fast_forward`, and `required_status_checks` with **one** context, `license/cla`
  (no `integration_id`, `strict_required_status_checks_policy: false`). Bypass list:
  `RepositoryRole` id 5 (admin), `bypass_mode: always`. (`gh api repos/getsphericon/sphericon/rulesets/17929791`)
- No classic branch protection on `main` (`gh api .../branches/main/protection` -> 404 "Branch not protected").
- Merge methods allowed: merge, squash and rebase all `true`. Squash defaults:
  `squash_merge_commit_title: COMMIT_OR_PR_TITLE`, `squash_merge_commit_message: COMMIT_MESSAGES`.
- Fork PR approval policy: `first_time_contributors`
  (`gh api .../actions/permissions/fork-pr-contributor-approval`). Default workflow token:
  `read`, `can_approve_pull_request_reviews: true`
  (`gh api .../actions/permissions/workflow`). `sha_pinning_required: false`
  (`gh api .../actions/permissions`).
- Workflows (`.github/workflows/`): `ci.yml` (trigger `pull_request` + `push: main`; jobs `lint`,
  `generated`, `commits`, `upgrade` (PR only), `tracker-size`, `test`, `test-e2e`, `test-frontend`,
  `test-frontend-coverage`, `compose-smoke`), `codeql.yml`, `nightly.yml`, `release-please.yml`,
  `scorecard.yml`. No `pull_request_target` or `workflow_run` anywhere. Actions are SHA-pinned.
  The `upgrade` job already passes title and body through env/file, not through the script.
- Existing `.github/` files: `pull_request_template.md` (Summary / Evidence / Merge Danger /
  Checklist), `dependabot.yml`, `actions/go-cache`. **No `CODEOWNERS`, no issue forms, no
  `copilot-instructions.md`, no `.github/instructions/`.** `CONTRIBUTING.md` and `AGENTS.md` exist at the root.
- The check jobs are all green-on-main check runs by the `github-actions` app (id 15368, from
  `gh api repos/.../commits/<sha>/check-runs`); `license/cla` is a commit **status**, not a check run.
- `cmd/upgradecheck` already implements `breaking` (title/body/commit `BREAKING CHANGE:` + `Upgrade:`
  rule), `migrations` (Squawk over added files) and `immutable` (history-collapse guard).
  `cmd/contractcopy` (`check:copies`, rule 6) already requires a non-empty reason on a
  `// contract-copy:` marker and explicitly has "no baseline file" (`CODING_STANDARDS.md`).

---

## 1. Rulesets vs classic branch protection

### 1.1 Differences that matter

- Rulesets have **no priority**: rules from all applicable rulesets and branch protection rules are
  aggregated and the most restrictive wins; only one classic protection rule applies to a branch,
  while several rulesets can target it. Anyone with read access can **view** active rulesets.
  Rulesets have `Active` / `Disabled` and an `Evaluate` status (used to dry-run metadata rules).
  Plan note on the page: rulesets are "for customers on GitHub Team and GitHub Enterprise plans";
  org-level rulesets need Enterprise. That sentence is about private repos: the page's gating
  reusable (`data/reusables/gated-features/repo-rules.md` in github/docs) says rulesets are available
  in "Public repositories on the Free plan for users and for organizations on the Free and Team plans"
  (summarised by the fetch tool, not a verbatim quote), so repo-level rulesets on our public repo under
  a free org are documented (re-checked 2026-10-23; also shown by the live "General" ruleset). Push
  rulesets are Team/Enterprise and private/internal only. No page says which individual rule types
  (1.2) are available on Free public repos.
  Source: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets
- Forks do **not** inherit rulesets from upstream; a ruleset applies to forks only when the fork is
  owned by the same organization. A new ruleset can only make rules more restrictive, never less.
  Source: https://docs.github.com/en/organizations/managing-organization-settings/creating-rulesets-for-repositories-in-your-organization
- Classic protection, by default, does not apply to admins; "Do not allow bypassing the above
  settings" extends it to admins and roles with the bypass permission. "Restrict who can push" always
  lets admins push. Bypass lists need an organization-owned repository.
  Source: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches

### 1.2 Rule inventory (ruleset names; REST schema is the authoritative list)

| Need                                                                                                                       | Ruleset rule                                                                                                                                                                                                                                                       | Source                                                                                                                                                                                        |
| -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Linear history                                                                                                             | `required_linear_history` (no params); repo must allow squash or rebase; blocks merge commits                                                                                                                                                                      | https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets , https://docs.github.com/en/rest/repos/rules       |
| PR required, approvals, code-owner review, last-push approval, stale-review dismissal, resolved threads, **merge methods** | `pull_request`: `required_approving_review_count` (0-10), `require_code_owner_review`, `require_last_push_approval`, `dismiss_stale_reviews_on_push`, `required_review_thread_resolution`, `allowed_merge_methods` (`merge`/`squash`/`rebase`)                     | https://docs.github.com/en/rest/repos/rules                                                                                                                                                   |
| Path-scoped reviewers without CODEOWNERS                                                                                   | `pull_request.required_reviewers[]` = `{reviewer, file_patterns, minimum_approvals}` (minimum 0 only adds the team, does not require approval). Availability by plan **unverified**.                                                                               | https://docs.github.com/en/rest/repos/rules                                                                                                                                                   |
| Required checks                                                                                                            | `required_status_checks`: `context` (required) + optional `integration_id` ("the optional integration ID that this status check must originate from"), `strict_required_status_checks_policy`, `do_not_enforce_on_create`                                          | https://docs.github.com/en/rest/repos/rules                                                                                                                                                   |
| Required workflows                                                                                                         | `workflows`: "Require all changes made to a targeted branch to pass the specified workflows before they can be merged"; items `repository_id`, `path`, `ref`, `sha`                                                                                                | https://docs.github.com/en/rest/repos/rules                                                                                                                                                   |
| Merge queue                                                                                                                | `merge_queue` (`merge_method`, `grouping_strategy` `ALLGREEN`/`HEADGREEN`, entry counts, timeouts)                                                                                                                                                                 | https://docs.github.com/en/rest/repos/rules                                                                                                                                                   |
| Commit message regex                                                                                                       | `commit_message_pattern` (`operator` `starts_with`/`ends_with`/`contains`/`regex`, `negate`, `pattern`). The docs page for creating rulesets describes "Restrict commit metadata" with regex "must match / must not match" and warns to use `\n?$` rather than `$` | https://docs.github.com/en/rest/repos/rules , https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository |
| Path/extension blocks                                                                                                      | `file_path_restriction` (`restricted_file_paths`, fnmatch, max 200 entries of 200 chars, supports allowed exceptions); push-ruleset only across the fork network                                                                                                   | https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets                                                     |
| Restrict updates / creations / deletions, block force pushes                                                               | "Restrict updates" = only bypass actors may push to matching refs; `non_fast_forward` blocks force pushes                                                                                                                                                          | same page                                                                                                                                                                                     |

**Correction (2026-10-23 re-check):** the earlier version said the "available rules" page does not
describe "require workflows", merge queue or "do not require status checks on creation". It does (the
first fetch was truncated; the github/docs source file has them). Quotes: "Applying this rule will
block direct pushes" (Require workflows to pass before merging: use it only on branches where every
change arrives through a pull request); merge queue: "This rule is not available for rulesets created
at the organization level"; status checks: select "Do not require status checks on creation" to allow
branch creation regardless of results. Plan availability of `workflows` and `merge_queue` on a Free
org's public repo is still **not stated** anywhere (**unverified**; only a live attempt to create the
rule would show it, which this research did not do: read-only). Same source:
https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets

`commit_message_pattern` ("Restrict commit metadata") is wrapped in an `repo-rules-enterprise` version
gate in the github/docs source and the page says "Organizations on a GitHub Enterprise plan can
access additional rules": treat it as **probably unavailable on our Free org** (not confirmed).
The two pages also disagree on squash merges (creating-rulesets: all commits on the branch must meet
the metadata requirements; available-rules: only the final merge commit is checked), so it is not
a title gate in any case. "Evaluate" enforcement status is mentioned on the docs only in the metadata
section and the insights page; no page says which plans have it (**unverified** for Free; do not plan
the rollout around it, use `Disabled` -> `Active` and check with a throwaway PR instead).
The "Restrict file paths" rule ("Prevent commits that include changes in specified file paths from
being pushed") is, by the REST rule type and the push-ruleset plan table, a push-ruleset feature
(Team/Enterprise, private/internal repos): this is an inference, not a page statement, and either way
it blocks paths outright rather than requiring review, so CODEOWNERS is the tool here.

### 1.3 Required status checks: how "it actually ran" is decided

- Required checks must pass on the **latest commit SHA**; earlier-commit results do not count.
  Successful states are `success`, `skipped` and `neutral`. If the test-merge commit has a status,
  that one must pass, otherwise the head commit.
  Source: https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks
- **A job skipped by an `if:` conditional reports success and satisfies the requirement.** A job
  that `needs` a failed job is skipped and "may not block merging"; the fix is `always()` with
  `needs`. (Same page.) Consequence for us: `upgrade` has `if: github.event_name == 'pull_request'`;
  fine for a PR, but never put a security-relevant `if:` on a required job.
- **A whole workflow skipped by `paths`, `paths-ignore`, `branches` filters** leaves its checks
  "Pending", blocking the PR ("Waiting for status to be reported"); the page says "Avoid requiring
  workflows that can be skipped." (Same page; and
  https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax : "If at least
  one path matches ... the workflow runs", otherwise it does not run and its checks stay Pending.)
  Our `ci.yml` has no path filters, which is the right shape; keep it that way.
- Workflow-created checks count toward a PR only when triggered by events such as `push`,
  `pull_request`, `pull_request_target`; a `workflow_dispatch` run on the head branch does not.
  (Troubleshooting page.)
- "If a check and a commit status have the same name, both must pass." A check can be bound to an
  app: with `integration_id`, a check not set by that app yields "Required status check "build" was
  not set by the expected GitHub App." (Troubleshooting page.) For Actions jobs the app is
  `github-actions` (id 15368, observed with `gh api` on our own commits), so `integration_id`
  stops a _third-party status_ from impersonating a check, but **not** another job named `lint`
  added by a PR's own workflow file. How GitHub resolves two workflows that both produce a job with
  the required name is **not covered by the troubleshooting page** (unverified); do not rely on it.
  Mitigate with section 3.4 (required workflow, or a CODEOWNERS gate on `.github/workflows/`).
- Merge queue: workflows must also trigger on `merge_group`, otherwise "the required status check
  will not be reported" and the merge fails; `merge_group` is separate from `pull_request`/`push`.
  Source: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue ,
  https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows
  The merge-queue page does not state plan/visibility availability (**still unverified**; it says
  the checks are "configured under branch protection rules or rulesets"); the REST schema lists a
  `merge_queue` ruleset rule, repo-level only. Merge queue is only worth it once `main` is busy; it is not needed to
  enforce the flow.

### 1.4 Bypass

- Ruleset bypass actors: repository admins, org owners, maintain/write roles, teams, GitHub Apps,
  Dependabot. `bypass_mode`: `always` (default), `pull_request` (the actor can bypass only by opening
  a PR; branch rulesets only), `exempt` (rules not evaluated, **no bypass audit entry**).
  Sources: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository ,
  https://docs.github.com/en/rest/repos/rules
- Our ruleset's admin bypass is `always`; the admin can push directly to `main` and merge past red
  checks. REST: "pull_request means that an actor can only bypass rules on pull requests"; the UI
  page: the actor "is now required to open a pull request to make changes to a repository" and "can
  then choose to bypass any branch protections and merge that pull request". So `pull_request` mode
  **removes the direct-push path for admins** (checks stay bypassable at merge on the PR, i.e. the
  admin can still merge a red PR with the bypass button; the page does not list which rules that
  skips: **unverified**). Never use `exempt` for humans.
- **Conflict with our own flow (gap in the first version):** `AGENTS.md` ("Land non-trivial work")
  has agents land with `git push origin HEAD:main` after local checks. That works today only because
  of admin bypass `always` (the live ruleset reports `current_user_can_bypass: always`; the only
  repo collaborator is the admin, no teams). Any of: required status checks that CI has not yet
  produced for a pushed SHA, a `pull_request` rule, a `workflows` rule ("block direct pushes"), or
  bypass mode `pull_request`, ends that flow for the maintainers. Pick one explicitly: (a) keep
  admin bypass `always` and accept that the gate binds only outside contributors (maintainers and
  their agents stay on direct pushes; this is what the ruleset does today for `license/cla`), or
  (b) `pull_request` mode and change `AGENTS.md` landing to "open a PR, wait for CI, bypass-merge or
  merge". Option (a) is the smallest change and still enforces everything on fork PRs, since fork
  authors are not admins. Required contexts must also exist on every commit pushed straight to
  `main` unless bypassed; `ci.yml` runs on `push: main`, which does not help a push that is
  rejected before the run exists.

---

## 2. CODEOWNERS as the human gate

- Locations: `.github/`, repo root, `docs/`; first found wins. One file per branch; **a PR uses the
  version on its base branch**. For a PR into the upstream repo (which is every fork PR) the upstream
  CODEOWNERS applies. Under 3 MB. Last matching pattern wins. gitignore syntax minus `\#`, `!` and
  `[ ]` ranges. Owners of one pattern must share a line.
  Source: https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
- Who counts: users and teams need explicit **write** access; teams must be visible; non-existent or
  under-privileged owners are silently not assigned. With required code-owner review, approval from
  **any one** owner satisfies a path (not all owners). Draft PRs request no owners until ready.
  Same page. A fork author is never an owner of upstream paths unless they hold write on upstream.
- Enforcement is `require_code_owner_review` in the `pull_request` rule (or classic "Require review
  from Code Owners"): "any pull request that modifies content with a code owner must be approved by
  that code owner."
  Source: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets
- Protect CODEOWNERS itself: "assign an owner to it or to the `.github/` directory" (about-code-owners).
  `docs.github.com` secure-use also advises adding `.github/workflows` to CODEOWNERS so changes
  "first require approval from a designated reviewer."
  Source: https://docs.github.com/en/actions/reference/security/secure-use
- Limits to plan around: (1) with a one-person maintainer set, the single owner is also the author
  of their own PRs; GitHub does not let an author's own approval count ("Pull request authors cannot
  approve their own pull requests", https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/reviewing-changes-in-pull-requests/approving-a-pull-request-with-required-reviews),
  and the repo today has exactly one collaborator (the admin) and no teams (`gh api`), so
  `require_code_owner_review` would block every maintainer PR until someone else approves or the
  admin bypasses (needs bypass mode `always`, or `pull_request` plus the PR bypass button). Create a `@getsphericon/maintainers` team with at least two members before requiring owner review
  on paths maintainers also edit, or the rule will block your own work (and you would then lean on
  the admin bypass). (2) Approvals are dismissed when the diff changes only if
  `dismiss_stale_reviews_on_push` is on. (3) Review is advisory about _content_: an owner can
  approve a bogus marker; CODEOWNERS only guarantees a named human looked.

Paths worth owning in this repo (proposal, not existing): `/.github/`, `/AGENTS.md`,
`/CODING_STANDARDS.md`, `/docs/agents/`, `/migrations/`, `/mise/`, `/.mise.toml`, `/mise.lock`,
`/.golangci.yml`, `/tools/analyzers/`, `/cmd/upgradecheck/`, `/cmd/contractcopy/`, `/oxlint.config.ts`,
`/knip.ts`, `/.gitleaks.toml`, `/schemalens.yml`, `/_typos.toml`, `/.testcoverage.yml`,
`/.gitattributes` (marks generated files), `/CLA.md`, `/ee/`.
`ee/` already has a policy (PRs only from people with a written agreement,
`CONTRIBUTING.md`) that CODEOWNERS can back mechanically.

---

## 3. CI for fork PRs

### 3.1 The three triggers

|                         | `pull_request`                                                                         | `pull_request_target`                                         | `workflow_run`                                                                             |
| ----------------------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Code / workflow context | merge branch of the PR (`GITHUB_REF` = merge branch, `GITHUB_SHA` = last merge commit) | **base repo default branch**                                  | default branch; only fires if the workflow file exists on the default branch               |
| Secrets on fork PRs     | **not passed** (except `GITHUB_TOKEN`)                                                 | available (page only warns attackers may "steal any secrets") | "able to access secrets and write tokens" even if the triggering workflow was unprivileged |
| `GITHUB_TOKEN`          | read-only for fork PRs                                                                 | read/write "even when it is triggered from a public fork"     | write-capable                                                                              |
| Fork-approval gate      | applies                                                                                | **"will always run, regardless of approval settings"**        | follows its trigger                                                                        |

Row-level provenance (re-checked): `pull_request` secrets/token rows and the merge-branch row are on
the events page ("With the exception of `GITHUB_TOKEN`, secrets are not passed to the runner when a
workflow is triggered from a forked repository"; "The `GITHUB_TOKEN` has read-only permissions in
pull requests from forks"). `pull_request_target`: "runs in the context of the default branch of the
base repository" (events page); the read/write token "even when it is triggered from a public fork"
is on workflow-syntax, not the events page; "regardless of approval settings" is on the org
disable/limit page only (the approving-workflow-runs page does not say it). The `workflow_run` column
was **not re-fetched** and stays as first written.

Sources: https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows ,
https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax (token rules),
https://docs.github.com/en/organizations/managing-organization-settings/disabling-or-limiting-github-actions-for-your-organization (approval, `pull_request_target`).

- Which commit's workflow _file_ a `pull_request` run uses is **not stated** on the events page or
  in workflow-syntax (both re-fetched 2026-10-23; the events page only says `GITHUB_SHA` is "the last
  merge commit of the pull request merge branch" and `GITHUB_REF` is "the merge branch"), nor on
  secure-use, which does not say a PR can modify the workflow that runs. **STILL UNVERIFIED on a
  primary page.** Secondary, tool-author statement: the `action-semantic-pull-request` README says
  `pull_request` "uses the latest configuration that is available in the current branch" while
  `pull_request_target` uses main's config (https://github.com/amannn/action-semantic-pull-request).
  Treat it as the working assumption (it matches how Actions is known to behave): **a fork PR can
  edit `.github/workflows/ci.yml` and the edited file is what runs under `pull_request`.** The docs'
  own advice is to be "especially alert to any proposed changes in the `.github/workflows/`
  directory" when approving runs
  (https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/approving-workflow-runs-from-public-forks).
- Official guidance: "Avoid using the `pull_request_target` and `workflow_run` workflow triggers
  with untrusted pull requests or code content"; treat artifacts from other workflows "with
  caution"; least-privilege `GITHUB_TOKEN`; intermediate env variable against script injection;
  pin actions to a full SHA (a lockfile supersedes pinning); never use self-hosted runners on
  public repos ("should almost never be used").
  Source: https://docs.github.com/en/actions/reference/security/secure-use

### 3.2 Approval gate for outside contributors

- Setting "Approval for running fork pull request workflows from contributors": (1) first-time
  contributors **new to GitHub**, (2) **first-time contributors**, (3) **all external contributors**.
  Approver needs write. Caveat on options 1 and 2: "a user that has had any commit or pull request
  merged into the repository will not require approval" (this sentence is on the org
  disable/limit page; the approving-workflow-runs page does not contain it) - so one merged typo fix gives an agent
  operator an auto-running workflow afterwards, including with a modified workflow file. Runs
  awaiting approval for over 30 days expire.
  Sources: https://docs.github.com/en/organizations/managing-organization-settings/disabling-or-limiting-github-actions-for-your-organization ,
  https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository ,
  https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/approving-workflow-runs-from-public-forks
- Ours is `first_time_contributors`. Moving to **all external contributors** costs a click per
  fork PR push but closes the workflow-edit hole until a human has looked at the diff (read the
  `.github/` part first). Settings that "Send write tokens" / "Send secrets" to fork PRs are
  documented for **private** repos only; on a public repo fork PRs get a read-only token and no
  secrets by default (same pages). The CI here needs no secrets on PRs, so keep it that way.

### 3.3 What is safe to run, tied to zizmor

zizmor audits (https://docs.zizmor.sh/audits/) that map directly onto fork-PR safety:
`dangerous-triggers` (flags `pull_request_target`, `workflow_run`, `issue_comment`),
`template-injection` (`${{ }}` in `run:`), `excessive-permissions`, `artipacked` (persisted checkout
credentials), `cache-poisoning` (cache writes from privileged events), `unpinned-uses`,
`ref-confusion`, `secrets-inherit`, `github-env` (GITHUB_ENV/PATH writes under dangerous triggers),
`bot-conditions` (spoofable `github.actor`), `overprovisioned-secrets`. We already run zizmor in
`mise run check` and every checkout uses `persist-credentials: false`; `unpinned-uses` page text was
truncated in my fetch (**unverified** detail).

Safe-by-construction pattern for our needs:

1. Everything that executes PR code (`mise run check`, tests, generators, `go run ./cmd/...`) stays
   on plain `pull_request`, `permissions: contents: read`, no secrets. This is already true.
2. A check that only reads **metadata** (PR title, PR body, author, the diff list) can run under
   `pull_request_target` _without checking out the PR head_ (so its code and config come from
   `main`), with `pull-requests: read`. Note the action's README says plain `pull_request` fails for
   fork PRs ("the GitHub token isn't available"), so `pull_request_target` is the only way for forks. This is the documented design of
   `amannn/action-semantic-pull-request`: trigger `pull_request_target` types
   `opened, reopened, edited, synchronize` (`synchronize` needed "if the workflow is required for
   merging"), permission `pull-requests: read`, rules `types`, `scopes`, `requireScope`,
   `subjectPattern`. Its README does not say whether it checks out PR code (check the action's
   source - **unverified**). zizmor will flag `pull_request_target` via `dangerous-triggers`; that is
   an audit finding to **justify in zizmor config with a reason**, not to silence silently
   (`AGENTS.md` forbids disabling lint rules; ask the owner).
3. Anything that must write back (comment, label) from a fork PR needs `pull_request_target` or
   `workflow_run` and must not run PR code. Prefer a job summary (works with read-only token) over
   comments.
4. Never use `pull_request_target` + `actions/checkout` of the head SHA ("Avoid using this event if
   you need to build or run code from the pull request", events page).

### 3.4 Making the enforcing checks non-bypassable by a PR that edits them

A PR can edit the workflow file, the `mise` tasks, `.golangci.yml`, `tools/analyzers`,
`cmd/upgradecheck` - anything the check runs. Ranked defences, strongest first:

1. **Ruleset `workflows` rule ("required workflows")**: the REST schema takes
   `{repository_id, path, ref, sha}` per workflow, i.e. the workflow file is referenced from a
   repository at a pinned ref/SHA, not from the PR (https://docs.github.com/en/rest/repos/rules).
   This is the platform's own mechanism for "run this regardless of what the PR's workflows say".
   The available-rules page does describe it (corrected, see 1.2): it "will block direct pushes", is
   for branches where every change arrives by PR, and ruleset workflows "can't use most CodeQL
   actions". Whether it works in a free-plan org and for a workflow in the **same** repo, which ref
   the run uses for fork PRs, and what the workflow can check out, is **still not stated anywhere
   fetched**: **unverified; test with a throwaway ruleset (Evaluate may not exist on Free) before
   relying on it.** Even so, the required workflow would run the PR's _code_ (`mise run
   check` over the PR's `.golangci.yml`) unless it checks out trusted tooling from `main` and only
   the PR's _source_ from the head.
2. **CODEOWNERS on every file that defines a check** (section 2) plus required code-owner review:
   a PR that edits `ci.yml`, `mise/tasks.toml`, `.golangci.yml`, analyzers, `cmd/upgradecheck`,
   ignore lists cannot merge without a maintainer. This is fully documented, works for fork PRs
   (upstream CODEOWNERS applies), and is the one defence that needs no workflow trick. It does not
   stop the _run_ on the PR (mitigated by "approve all external contributors", 3.2).
3. **Metadata checks under `pull_request_target`** (3.3 item 2): their config comes from `main`,
   so a PR cannot weaken them in its own run.
4. **Reusable workflow pinned by SHA** called from the PR's workflow: the callee version is fixed
   (reusable workflows can be referenced by SHA, tag or branch, per
   https://docs.github.com/en/actions/reference/workflows-and-actions/reuse-workflows) but the
   _caller_ is the PR's file, so a PR can simply delete the call. Alone this does **not** protect;
   it is only useful combined with (1) or (2). (The page says nothing about forks: **unverified**.)
5. `strict_required_status_checks_policy` (up-to-date branch) does not help here.

Also required: list the check contexts in the ruleset (`lint`, `generated`, `commits`, `upgrade`,
`test`, `test-e2e`, `test-frontend (1)`, `test-frontend (2)`, `test-frontend (3)` (the matrix job's
context includes the shard, verified in `gh api commits/main/check-runs`),
`test-frontend-coverage`, `tracker-size`, `compose-smoke`; `chart`/`goreleaser`/`release-please`/
`analysis` also appear on main but belong to other workflows, do not require them; CodeQL's
`analyze` runs on `pull_request` and schedule only) with `integration_id: 15368` so no status from another source can stand in; use a
single aggregating job (`needs: [...]`, `if: always()`, fails unless every need succeeded) as the
one required context so job renames or the matrix do not silently drop a requirement. Skipped
dependencies are the classic hole (1.3).

---

## 4. PR-level guards, templates, instruction files

### 4.1 Title (Conventional Commits) and squash behaviour

- Release-please reads **commits on `main`**; its docs page fetched does not say PR titles drive
  notes (https://github.com/googleapis/release-please/blob/main/docs/manifest-releaser.md says the
  changelog comes from commit history; no squash/PR-title statement). Therefore the _squash
  commit subject_ is what matters.
- GitHub's default squash message: "uses the commit title and message if the pull request contains
  only 1 commit", otherwise the PR title and a commit list. The alternatives are "Pull request
  title" only, title + commit details, title + description.
  Source: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/configuring-commit-squashing-for-pull-requests
  **Our repo is on `COMMIT_OR_PR_TITLE` (the default)**: a one-commit PR is merged with its commit
  message, not its title, so a title check alone is not enough. Fix in settings: set the squash
  default to "Pull request title" (REST: `squash_merge_commit_title: PR_TITLE`), so the title the
  CI validated is the commit release-please reads. Alternatively validate both (we already run
  commitlint on the commits in the `commits` job, and `upgrade` validates title and body).
- Title validation: `amannn/action-semantic-pull-request` (see 3.3). Or extend `cmd/upgradecheck`
  with a `title` subcommand (it already receives `PR_TITLE`) and run it in the metadata workflow.
- A ruleset `commit_message_pattern` regex applies to commits **pushed to the branch**; with
  squash-only merges on a PR, whether it sees the generated squash message is not stated in the
  pages fetched (**unverified**), so do not use it as the title gate.
- Squash-only also yields linear history; add `required_linear_history` anyway (it blocks merge
  commits, and "the repository must allow squash or rebase merging first"). `AGENTS.md` bans merge
  commits; rules: `pull_request.allowed_merge_methods = [squash]` and disable merge-commit and
  rebase in repo settings.

### 4.2 PR body requirements

- GitHub has **no** native "required PR body field". Templates are prefilled text only:
  `.github/pull_request_template.md`, `docs/`, root, or a `PULL_REQUEST_TEMPLATE/` dir with
  `?template=` query; available once on the default branch. The docs do not say templates apply to
  PRs opened from forks, via `gh pr create --body`, or the API, nor that they are enforced
  (**unverified**; the text describes them as content contributors "automatically see").
  Source: https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/creating-a-pull-request-template-for-your-repository
  An agent that opens a PR through the API/`gh` with its own body bypasses the template.
- So a required line (`Upgrade:` for breaking, a declared-markers line) must be **checked in CI**
  from `github.event.pull_request.body`. We do this already for `BREAKING CHANGE:`/`Upgrade:`
  (`upgrade` job, `cmd/upgradecheck`). The body can be edited after the run: a body edit triggers
  `pull_request` `edited` only if the workflow lists `types: [..., edited]`. Our `ci.yml` has
  `on: pull_request:` with **default types**, so editing the body after the first green run does not
  re-run `upgrade`. Confirmed on primary pages: "By default, a workflow only runs when a
  `pull_request` event's activity type is `opened`, `synchronize`, or `reopened`" (events page), and
  the webhook doc for `edited`: "The title or body of a pull request was edited, or the base branch
  of a pull request was changed." Add `edited` for the job that reads the title/body. Caveat: the
  `types:` list is per workflow, so adding `edited` to `on: pull_request` in `ci.yml` re-runs **all**
  jobs of CI on every body edit; a separate small workflow (or the `pull_request_target` metadata
  workflow of rec 5) avoids that.

### 4.3 CONTRIBUTING, issue forms

- CONTRIBUTING (root, `docs/` or `.github/`): GitHub shows a link to it when someone opens a PR or
  issue and on the repo's contribute page.
  Source: https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/setting-guidelines-for-repository-contributors
  That is a link, not a gate. Ours exists.
- Issue forms: **not fetched**; no claim made (**unverified**). They structure issues, not PRs.

### 4.4 Which agents read what (advisory only)

| Agent                      | Files read                                                                                                                                                                          | Source                                                                                               |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| GitHub Copilot cloud agent | `.github/copilot-instructions.md`, `.github/instructions/*.instructions.md` (`applyTo` globs; `excludeAgent`), `AGENTS.md` (nearest wins), or a single root `CLAUDE.md`/`GEMINI.md` | https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions |
| Copilot code review        | repo-wide + path-specific instructions only, read from the **PR head branch** (so a PR can edit the reviewer's instructions)                                                        | same page                                                                                            |
| Claude Code                | `CLAUDE.md`; `AGENTS.md` only when no `CLAUDE.md` exists above the cwd (v2.1.277+); "treats them as context, not enforced configuration"; enforcement = hooks                       | https://code.claude.com/docs/en/memory                                                               |
| Codex                      | `AGENTS.md` (override file, then base) from the repo root down to cwd, one per directory, capped at 32 KiB by default                                                               | https://learn.chatgpt.com/docs/agent-configuration/agents-md                                         |
| Cursor                     | `AGENTS.md` and `.cursor/rules/*.mdc`; only dashboard Team Rules can be "enforced"; docs say "AI guidance should not be your only security control"                                 | https://cursor.com/docs/context/rules                                                                |

All advisory. A fork contributor's agent reads the instructions in _their_ checkout, which can
differ from ours, and nothing forces the agent to obey them. Practical consequences: keep
`AGENTS.md` short, put the one-paragraph "what CI will reject and why" at the top, and add
`.github/copilot-instructions.md` only if we want Copilot to see something different from
`AGENTS.md` (it already reads `AGENTS.md`).

---

## 5. Controlling an agent's own PRs

Copilot cloud agent (documented behaviour):

- Pushes to one branch only: the existing PR's branch, otherwise a new `copilot/` branch; it
  "cannot directly run `git push` or other Git commands"; "Branch protections and required checks
  still apply"; it opens **draft** PRs and "cannot approve or merge." If a ruleset or protection
  rule is incompatible with the agent, "access to the agent is blocked"; admins can add Copilot as a
  bypass actor (these two quotes are on the about-coding-agent page only, not on risks-and-mitigations).
  Sources: https://docs.github.com/en/copilot/concepts/agents/coding-agent/risks-and-mitigations ,
  https://docs.github.com/en/copilot/concepts/agents/coding-agent/about-coding-agent
- Workflows do **not** run on the agent's pushes until a user with write clicks **Approve and run
  workflows** (admins may opt into automatic runs). The page warns to be careful with changes to
  `.github/workflows/`. The user who asked for the PR cannot approve it, and the approval does not
  count toward a required number of approvals.
  Sources: https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/review-copilot-prs ,
  risks-and-mitigations page above. (Do not enable the "run automatically" option.)
- Firewall: GitHub "restricts Copilot cloud agent's access to the internet"; the default allowlist
  is on a different page (not fetched, **unverified**).
- `copilot-setup-steps.yml`: `.github/workflows/copilot-setup-steps.yml`, single job named exactly
  `copilot-setup-steps`, must exist on the default branch; only `steps`, `permissions`, `runs-on`,
  `services`, `snapshot`, `timeout-minutes` (max 59) take effect; a failing setup step is skipped
  over, not fatal. It also runs as an ordinary workflow when edited.
  Source: https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-environment
  We could ship one that runs `mise install` + `pnpm install` so the agent can run `mise run check`
  itself; that is quality-of-life, not enforcement.
- Prompt-injection mitigations: hidden characters filtered, only write-access users' comments reach
  the agent. (risks-and-mitigations page.)
- Third-party agents (Claude Code, Codex, Cursor, others) open PRs as the contributor's own account
  or an App, from forks or branches; GitHub documents **nothing** specific for them beyond the normal
  fork rules. Their PRs are governed by the same ruleset, approval gate and CODEOWNERS as any human
  PR. An agent run from a GitHub App/bot account is not treated as a first-time contributor
  differently in anything I could confirm (**unverified**).

---

## 6. Escape-hatch governance (`// contract-copy: <reason>` and similar)

Problem: `cmd/contractcopy` accepts any non-empty reason, `golangci-lint` analyzers and ignore
lists may accept markers too, and the PR author controls the diff. Options, with what GitHub
provides and what is our own code:

| Option                                                                                                                                                                                                                                                                  | GitHub feature involved                                                                                   | Own code needed                                                                                                                                                              |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A. Markers live in one registry file (for example `contractcopy.allow.yml`: file, value, reason), the checker reads **only** that file, and the file is a CODEOWNERS path with required code-owner review                                                               | CODEOWNERS + `require_code_owner_review` (section 2)                                                      | Change `cmd/contractcopy` to read the registry. This reverses the current "no baseline file" decision in `CODING_STANDARDS.md` - needs the owner's say-so.                   |
| B. Markers stay inline, but a CI job computes the markers **added in the diff** (`git diff base...head`, grep the marker, also changed ignore lists/`nolint`-style config) and lists them in the job summary; fails when there are markers unless an owner has approved | `pull_request` (metadata + diff, no secrets); job summary needs no write token; approval is the PR review | New small subcommand (for example `cmd/upgradecheck markers` or a new `cmd/escapes`). The "approved" signal must be a _review state_, not a PR-body line (below).            |
| C. Fail unless the PR **body** carries a declared line (`Contract-copy: reviewed`)                                                                                                                                                                                      | Nothing native; body read via `github.event.pull_request.body`                                            | The body is written by the author and checked for text only, so a bogus declaration passes. It forces visibility, not truth. Use only as a reviewer prompt on top of A or B. |
| D. A `CODEOWNERS` entry cannot be scoped to "lines containing a marker"; it is per path                                                                                                                                                                                 | n/a (per docs, patterns are path globs)                                                                   | -                                                                                                                                                                            |
| E. Label gate (`escape-approved`) required by CI                                                                                                                                                                                                                        | Applying labels needs triage+ permission (**unverified** on a primary page)                               | Workflow reading labels; pair with `pull_request_target` or `labeled` trigger                                                                                                |

Recommended shape: A for markers that are genuinely allow-lists (contract copies, ignores), because
it makes "a human approved the reason" a platform-enforced fact via CODEOWNERS; B as a cheap
fallback for inline markers (also catches `//nolint`, `oxlint-disable`, `@ts-expect-error`,
`squawk-ignore`, `knip` ignores if we want, though `AGENTS.md` already forbids disabling rules).
The existing `-- squawk-ignore <rule>` escape in migrations is the same class: put the diff listing
into the same job and CODEOWNERS-own `/migrations/`. Quality of the reason text itself (is it
true?) is not machine-checkable (section 7).

---

## 7. What GitHub cannot enforce

- That a contributor or agent **read** `AGENTS.md`, `CONTRIBUTING.md` or Copilot instructions, or
  ran `mise run check` / `mise run generate` locally. Instruction files are context (Claude Code docs:
  "context, not enforced configuration"; Cursor: guidance "should not be your only security
  control"). Only CI results on the pushed commit are evidence.
- That a PR **body is truthful**, or that required body fields exist: templates are prefilled text,
  and the docs do not claim enforcement (4.2). Only regex/format checks in CI are possible, and a
  marker reason or "Evidence" section can be invented.
- That a marker's reason is **correct**; only that a human with a CODEOWNERS entry approved it.
- That a PR **cannot edit the workflow** that checks it, unless the check runs from `main`
  (`pull_request_target` metadata jobs, a `workflows` ruleset rule - unverified here) or a code
  owner must approve (3.4).
- Anything about the agent's **local** environment, tools, model, or prompt; GitHub only sees the
  commits and PR it pushes. Copilot's own agent is the exception: GitHub controls its sandbox and
  firewall (section 5).
- A skipped or filtered required check is treated as passing when it is a skipped _job_, and as
  pending when it is a skipped _workflow_ (1.3): GitHub does not tell you "this check never
  evaluated the thing you wanted".
- Anything for a **fork's own** repo rules: forks do not inherit upstream rulesets (1.1).

---

## 8. Recommendations for getsphericon/sphericon, ranked

Legend: EXISTS = already in the repo/GitHub today; NEW = to add.

1. **NEW (settings, high leverage, no code): turn the ruleset into the real gate.** On the existing
   "General" ruleset add: `required_status_checks` for the CI jobs above (each with
   `integration_id` 15368; `license/cla` stays as is), `pull_request` with
   `allowed_merge_methods: [squash]`, `require_code_owner_review: true`,
   `dismiss_stale_reviews_on_push: true`, `required_review_thread_resolution: true`,
   `required_linear_history`. **Decide the admin bypass first (changed after verification):** keep
   `always` if maintainers and agents keep landing with `git push origin HEAD:main` (the gate then
   binds fork PRs, which is the goal); `pull_request` mode would end that flow and requires rewriting
   the `AGENTS.md` landing section (1.4). With a one-person owner set, `require_code_owner_review`
   blocks the maintainer's own PRs (authors cannot approve their own PRs), so add a second owner/team
   or rely on the admin bypass. Keep `deletion` and `non_fast_forward` (EXISTS). Use one aggregator
   job as the single required context (NEW workflow job). `Evaluate` availability on Free is
   unverified: roll out with a throwaway PR instead. Do not rely on `commit_message_pattern` (likely
   Enterprise-only). (Sections 1.2-1.4.)
2. **NEW: `.github/CODEOWNERS`** covering the paths in section 2 (workflows, `AGENTS.md`,
   `CODING_STANDARDS.md`, `docs/agents/`, `migrations/`, `mise/`, check configs, analyzers,
   `cmd/upgradecheck`, `cmd/contractcopy`, `.gitattributes`, `CLA.md`, `ee/`), owned by a team of at
   least two maintainers, and the file protected by an owner entry. This is the only documented
   way to stop a PR from weakening the check that judges it.
3. **NEW (setting): fork approval policy to "all external contributors"** (currently
   `first_time_contributors`), so one merged fix does not buy auto-run of edited workflows. Do not
   enable "run Copilot workflows automatically".
4. **NEW (setting): squash default = "Pull request title"** and disable merge-commit and rebase
   merges; today `COMMIT_OR_PR_TITLE` lets a one-commit PR bypass the title the CI checked.
5. **NEW workflow: PR-metadata checks under `pull_request_target`** (no checkout of PR head,
   `permissions: pull-requests: read`, types `opened, reopened, edited, synchronize`): Conventional
   Commit title (`amannn/action-semantic-pull-request`, or a `title` subcommand of `cmd/upgradecheck`)
   and the body rules. Config comes from `main`, so the PR cannot edit it. Needs a justified
   zizmor `dangerous-triggers` entry (owner approval: do not disable rules silently).
   Also add `types: [opened, synchronize, reopened, edited]` to the existing `upgrade` job, which
   today does not re-run when the body is edited. EXISTS: `commits` (commitlint) and `upgrade`
   (`BREAKING CHANGE:`/`Upgrade:`, Squawk, immutability) in `ci.yml`.
6. **NEW: escape-hatch governance** (section 6): move `contract-copy` markers (and any other allow
   list) into a CODEOWNERS-protected registry (option A) or add the diff-listing job (option B).
   EXISTS: reason-required marker in `cmd/contractcopy`; there is no registry and no reviewer gate.
   Requires the owner's decision since `CODING_STANDARDS.md` says "no baseline file".
7. **EXISTS, keep: `pull_request` for everything that runs PR code**, `contents: read`,
   `persist-credentials: false`, SHA-pinned actions, no secrets, no path filters, zizmor in
   `mise run check`, CLA assistant (`license/cla`). Add the repo setting "require actions pinned to a
   full SHA" (`sha_pinning_required` is currently `false`;
   https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository)
   - NEW, small. Check that reusable-workflow tag references stay allowed (the page says they can).
8. **TRY, verify first: ruleset `workflows` (required workflows)** for the aggregator/metadata
   workflow, pinned to a SHA/ref (REST schema in 1.2). Unverified for a free-plan org and a
   same-repo workflow; test on a throwaway ruleset in `Evaluate` mode. If it works it replaces the
   dependence on item 2 for checks that must run from trusted code. Documented side effect: it
   "will block direct pushes", so it forces the bypass decision of item 1.
9. **NEW, cheap, advisory: contributor-facing text.** Add a short "CI will reject" section at the
   top of `AGENTS.md`/`CONTRIBUTING.md` (generated code, titles, markers, `Upgrade:` line), extend
   `.github/pull_request_template.md` with a `Contract-copy / escape hatches` and `Upgrade:`
   placeholder (visibility only, item 6 option C), and optionally
   `.github/copilot-instructions.md` and a `copilot-setup-steps.yml` that runs `mise install` and
   `pnpm install`. EXISTS: `AGENTS.md`, `CONTRIBUTING.md`, `pull_request_template.md`. All advisory
   (4.4); issue forms are optional and unverified here.
10. **Defer: merge queue** (`merge_queue` rule + `merge_group` trigger in every required workflow).
    Not needed to enforce the flow; adopt only when `main` contention makes "up to date" costly.
    Availability by plan **unverified**.

Open points to confirm on github.com UI or with the owner before acting: whether the
`workflows` rule and `merge_queue` are available to this org; whether `Evaluate` exists on Free;
whether a `pull_request` run uses the PR's own workflow file (no primary statement found);
whether the admin bypass stays `always` (direct-push landing flow); a second code owner so the
maintainer's PRs are not blocked; whether to reverse "no baseline file". (Resolved by the
re-check: default `pull_request` types, rulesets on Free public repos, author cannot self-approve.)

---

## Verification log (2026-10-23, adversarial re-check)

Method: every cited page re-fetched; repo facts re-read from `.github/workflows/ci.yml`,
`mise/tasks.toml`, `CODING_STANDARDS.md`; live state via read-only `gh api` GET.
Fetch-tool output is paraphrased in places; quotes below are what the tool returned as verbatim.

| Claim                                                                                                                                                                          | Verdict                                                                                         | Evidence                                                                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Live state in section 0 (ruleset, bypass always, plan free, merge methods, squash defaults, approval policy, token read, `sha_pinning_required` false, no CODEOWNERS)          | CONFIRMED                                                                                       | `gh api repos/getsphericon/sphericon/rulesets/17929791`, `.../actions/permissions*`, `repos/...` and `orgs/getsphericon`; `.github/` has no CODEOWNERS                                                                                        |
| `license/cla` is a status, jobs are check runs from app 15368                                                                                                                  | CONFIRMED                                                                                       | `gh api commits/main/status` and `check-runs`                                                                                                                                                                                                 |
| ci.yml job names, no path filters, `upgrade` has `if: github.event_name == 'pull_request'`, `on: pull_request:` default types                                                  | CONFIRMED                                                                                       | `.github/workflows/ci.yml` lines 3-8, 116; matrix contexts are `test-frontend (1                                                                                                                                                              | 2   | 3)` (note now lists them) |
| `upgrade` reads PR title/body from `github.event.pull_request.*` via env/file                                                                                                  | CONFIRMED                                                                                       | ci.yml lines 139-142, `mise/tasks.toml` `check:upgrade` (`PR_TITLE`, `PR_BODY_FILE`)                                                                                                                                                          |
| "no baseline file" in `CODING_STANDARDS.md`                                                                                                                                    | CONFIRMED                                                                                       | `CODING_STANDARDS.md` lines 53, 60                                                                                                                                                                                                            |
| Rulesets available to a free org's public repo                                                                                                                                 | CONFIRMED (was unverified)                                                                      | github/docs `data/reusables/gated-features/repo-rules.md`: "Public repositories on the Free plan ... organizations on the Free and Team plans" (paraphrase by fetch tool); live ruleset exists                                                |
| "Available rules" page lacks `workflows`, merge queue, do-not-require-on-creation                                                                                              | CORRECTED                                                                                       | github/docs source has all three; "Applying this rule will block direct pushes"                                                                                                                                                               |
| `workflows` rule works on Free / same repo / for fork PRs                                                                                                                      | STILL UNVERIFIED                                                                                | REST: "Require all changes made to a targeted branch to pass the specified workflows"; no plan statement on any page                                                                                                                          |
| Merge queue plan availability                                                                                                                                                  | STILL UNVERIFIED                                                                                | managing-a-merge-queue page says nothing on plans; "not available for rulesets created at the organization level"                                                                                                                             |
| `Evaluate` mode on Free                                                                                                                                                        | STILL UNVERIFIED                                                                                | docs mention it only in the metadata section; "rule insights dashboard is available for GitHub Team and GitHub Enterprise Cloud"                                                                                                              |
| `commit_message_pattern` usable on Free                                                                                                                                        | STILL UNVERIFIED (likely not)                                                                   | github/docs source gated `repo-rules-enterprise`; "Organizations on a GitHub Enterprise plan can access additional rules"                                                                                                                     |
| Squash + metadata rule sees the PR title                                                                                                                                       | STILL UNVERIFIED, and the two pages contradict each other                                       | creating-rulesets: "If you squash merge a branch, all commits on that branch must meet any metadata requirements"; available-rules: "only the final merge commit is checked"                                                                  |
| `bypass_mode` values                                                                                                                                                           | CONFIRMED                                                                                       | REST: `always`, `pull_request` ("can only bypass rules on pull requests"), `exempt` ("rules will not be run for that actor and a bypass audit entry will not be created"); the UI page names only two modes                                   |
| Bypass `pull_request` effect on direct push                                                                                                                                    | CORRECTED / ADDED                                                                               | first version missed that it ends the direct-push landing flow; UI page: "is now required to open a pull request"                                                                                                                             |
| Default `pull_request` types                                                                                                                                                   | CONFIRMED (was unverified)                                                                      | "By default, a workflow only runs when a `pull_request` event's activity type is `opened`, `synchronize`, or `reopened`." events page                                                                                                         |
| `edited` fires on body/title edit                                                                                                                                              | CONFIRMED                                                                                       | webhooks page: "The title or body of a pull request was edited, or the base branch of a pull request was changed." (events page does not explain it)                                                                                          |
| Skipped job via `if:` reports success; path-filtered workflow stays Pending; latest SHA; success/skipped/neutral; check+status same name; "not set by the expected GitHub App" | CONFIRMED                                                                                       | troubleshooting-required-status-checks (quotes in 1.3); two jobs with the same name: page silent, STILL UNVERIFIED                                                                                                                            |
| `integration_id` semantics                                                                                                                                                     | CONFIRMED                                                                                       | REST: "The optional integration ID that this status check must originate from." Using id 15368 for all Actions jobs: inferred from `gh api` data                                                                                              |
| CODEOWNERS: PR uses base-branch file, fork PRs use upstream's                                                                                                                  | CONFIRMED                                                                                       | about-code-owners: "pull requests use the version of `CODEOWNERS` from the base branch of the pull request"                                                                                                                                   |
| CODEOWNERS needs write access; protect the file itself                                                                                                                         | CONFIRMED                                                                                       | "must have write permissions"; "you also need to define an owner for the CODEOWNERS file itself"                                                                                                                                              |
| Author's own approval counts                                                                                                                                                   | CORRECTED (was unverified): it does not                                                         | "Pull request authors cannot approve their own pull requests." Impact: owner review blocks the sole maintainer's PRs                                                                                                                          |
| `pull_request` fork: no secrets, read-only token                                                                                                                               | CONFIRMED                                                                                       | events page quotes in 3.1                                                                                                                                                                                                                     |
| `pull_request` runs the PR's own (fork-edited) workflow file                                                                                                                   | STILL UNVERIFIED on a primary page                                                              | events, workflow-syntax and secure-use do not say it; only the amannn README (secondary) and the approving-runs advice about `.github/workflows/` changes                                                                                     |
| `pull_request_target`: base-branch context; always runs regardless of approval                                                                                                 | CONFIRMED (sources re-attributed)                                                               | events: "runs in the context of the default branch of the base repository"; org page: "will always run, regardless of approval settings" (approving-runs page does not say it)                                                                |
| `pull_request_target` token read/write on public forks                                                                                                                         | CONFIRMED                                                                                       | workflow-syntax: "granted read/write repository permission, even when it is triggered from a public fork" (not on events page)                                                                                                                |
| `workflow_run` column of the trigger table                                                                                                                                     | NOT RE-FETCHED                                                                                  | left as first written                                                                                                                                                                                                                         |
| Fork approval options and merged-commit caveat; 30-day expiry; write access to approve                                                                                         | CONFIRMED                                                                                       | org disable/limit page (options, caveat); approving-runs page (30 days, write access, "especially alert to any proposed changes in the `.github/workflows/` directory")                                                                       |
| Send write tokens/secrets only for private repos                                                                                                                               | CONFIRMED                                                                                       | "Available to private repositories only"                                                                                                                                                                                                      |
| secure-use advice (avoid `pull_request_target`/`workflow_run` with untrusted code; CODEOWNERS on workflows)                                                                    | CONFIRMED                                                                                       | quotes: "Avoid using the `pull_request_target` and `workflow_run` workflow triggers with untrusted pull requests or code content."; "any proposed changes to these files will first require approval from a designated reviewer"              |
| Reusable workflows can be pinned by SHA                                                                                                                                        | NOT RE-FETCHED; secure-use gives no pinning rule for reusable workflows                         | unchanged                                                                                                                                                                                                                                     |
| amannn: `pull_request_target` types, `synchronize` for required, `pull-requests: read`                                                                                         | CONFIRMED; ADDED: plain `pull_request` errors for fork PRs ("the GitHub token isn't available") | README                                                                                                                                                                                                                                        |
| amannn does not check out PR code                                                                                                                                              | STILL UNVERIFIED                                                                                | README silent; action source not read                                                                                                                                                                                                         |
| Squash default `COMMIT_OR_PR_TITLE` behaviour                                                                                                                                  | CONFIRMED                                                                                       | "The default commit message presented to contributors when merging is the commit title and message" for one commit; PR title + commit list for 2 or more                                                                                      |
| Release-please uses commit messages, not PR titles                                                                                                                             | STILL UNVERIFIED                                                                                | release-please page not re-fetched; consistent with repo `AGENTS.md` ("repository squashes with the PR title and the commit messages")                                                                                                        |
| PR templates not enforced / not applied via API                                                                                                                                | STILL UNVERIFIED                                                                                | not re-fetched                                                                                                                                                                                                                                |
| Issue forms                                                                                                                                                                    | STILL UNVERIFIED                                                                                | not fetched; out of scope for PR enforcement                                                                                                                                                                                                  |
| Label permissions (triage+)                                                                                                                                                    | STILL UNVERIFIED                                                                                | not fetched                                                                                                                                                                                                                                   |
| Copilot agent: copilot/ branch, no `git push`, rules apply, draft PR, cannot approve/merge, workflows need approval, requester cannot approve                                  | CONFIRMED                                                                                       | risks-and-mitigations quotes in section 5; review-copilot-prs: "your approval of a Copilot pull request won't count toward the required number"                                                                                               |
| Copilot blocked by incompatible ruleset; admin can add bypass actor                                                                                                            | CONFIRMED (source corrected to about-coding-agent)                                              | "If a ruleset or branch protection rule is incompatible with Copilot cloud agent, access to the agent is blocked."                                                                                                                            |
| Copilot reads AGENTS.md / CLAUDE.md / copilot-instructions.md; code review reads head branch                                                                                   | CONFIRMED                                                                                       | add-repository-instructions: "from the head branch"                                                                                                                                                                                           |
| Claude Code reads AGENTS.md only without CLAUDE.md, v2.1.277+, "context, not enforced configuration"                                                                           | CONFIRMED                                                                                       | code.claude.com memory page                                                                                                                                                                                                                   |
| Codex AGENTS.md walk and 32 KiB cap                                                                                                                                            | CONFIRMED                                                                                       | learn.chatgpt.com agents-md page                                                                                                                                                                                                              |
| Cursor reads AGENTS.md; "AI guidance should not be your only security control"                                                                                                 | CONFIRMED                                                                                       | cursor.com rules page                                                                                                                                                                                                                         |
| Copilot firewall allowlist, third-party agent bot treatment                                                                                                                    | STILL UNVERIFIED                                                                                | not fetched / not documented                                                                                                                                                                                                                  |
| Gaps checked and judged: "approval of most recent reviewable push", "Restrict updates/creations", code scanning, tag rules                                                     | Not added to recommendations                                                                    | `require_last_push_approval` exists (available-rules: "require an approval from someone other than the last person to push to a branch"); useful only with 2+ maintainers; tag/code-scanning rules are out of scope for the contribution flow |
