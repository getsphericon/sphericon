# Where a public open-core repository should keep its engineering documents: GitHub's conventions and general practice

Researched 2026-10-23 against primary sources only: docs.github.com pages and the `github/docs` source they are
built from (`content/` and `data/reusables/`), the files and directory listings of first-party GitHub repos and of
mature open-source projects read through the GitHub contents API (read-only GET), and the home pages of the
documentation and ADR conventions themselves (Diataxis, matklad, Nygard, MADR, Google's documentation guide, Write
the Docs, agents.md, Claude Code docs). A blog was used at most to find a repo. Every claim cites the page or file;
anything not confirmed on a page fetched during this research is marked **unverified**. Directory listings were read
on the default branch on the research date and change over time. A `WebFetch` answer is a summary of the page by a
small model, so where a claim carries weight it was re-read in the raw `github/docs` source and that path is cited.

Companion notes: [`agent-instructions-in-contributing-guides.md`](agent-instructions-in-contributing-guides.md)
(what other projects tell agents) and [`enforcing-the-contribution-flow-on-github.md`](enforcing-the-contribution-flow-on-github.md)
(rulesets, CODEOWNERS, fork-safe CI). This note only decides _where_ our documents live.

**Short answer.**

1. GitHub itself only treats a small, fixed set of files specially (README, CONTRIBUTING, CODE_OF_CONDUCT, LICENSE,
   SECURITY, SUPPORT, issue/PR templates, CODEOWNERS, FUNDING, ACCESSIBILITY), and for those it accepts three
   locations in a fixed order: `.github/`, then the repository root, then `docs/`. `ARCHITECTURE.md`,
   `CODING_STANDARDS.md`, `GLOSSARY.md` and ADRs are not mentioned anywhere on the pages read, so GitHub
   imposes no location for them. (An absence claim, limited to the pages listed in section 1.)
2. GitHub's own guidance on longer documentation is "README short, long documentation elsewhere" (a wiki, in its
   words), and its own docs repo moved most of its `contributing/` folder (15 of 22 files) to the published site, keeping
   in the repo only process and environment files.
3. In every mature project surveyed (Kubernetes, Rust, Go, Django, Grafana, GitLab, rust-analyzer, plus seven
   first-party GitHub repos) the root carries a short set of entry points (README, CONTRIBUTING or a pointer to
   it, licence, conduct, security, and increasingly `AGENTS.md`), while coding standards, architecture and
   contributor guides live under a documentation directory (`docs/`, `doc/development/`, `contribute/`,
   `docs/internals/contributing/`) or in a separate guide. **No surveyed repo keeps a root `CODING_STANDARDS.md`
   or a root `ARCHITECTURE.md`**, including rust-analyzer, whose maintainer wrote the article that popularised
   `ARCHITECTURE.md`.
4. Practice is split on: `CONTRIBUTING.md` at root vs `.github/`; architecture and standards in-repo vs a separate
   guide; the name of the directory (`docs/`, `doc/`, `contribute/`); and ADR directory name (`docs/adr`,
   `docs/adrs`, `docs/decisions`, `doc/arch`). It is not split on "thin root, rest under a documentation
   directory", which is the best-supported layout (section 8).
5. Agent loading does not favour any location for long standards: nothing auto-loads `docs/`; agents read short
   instruction files (`AGENTS.md`, nested `AGENTS.md`) and follow pointers on demand.

---

## 1. Where GitHub looks for and surfaces community health files

| Fact                                                                                                                                                                                                                                                                                                                                                                                               | Source                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Default community health files "such as CONTRIBUTING and CODE_OF_CONDUCT" can be put in an org's `.github` repository; they apply to "any repository owned by the account that does not contain its own file of that type". The `.github` repo must be public (internal for EMU orgs).                                                                                                             | [Creating a default community health file](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file); source `content/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file.md` ([github/docs](https://github.com/github/docs/blob/HEAD/content/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file.md)) |
| Supported default files: ACCESSIBILITY.md, CODE_OF_CONDUCT.md, CONTRIBUTING.md, discussion category forms, FUNDING.yml, issue and PR templates with `config.yml`, SECURITY.md, VULNERABILITY_REPORT.yml/.yaml, SUPPORT.md. **GOVERNANCE, ARCHITECTURE, CODING_STANDARDS and AGENTS.md are not in the list.**                                                                                       | same page, "Supported file types" table (source lines 58-78)                                                                                                                                                                                                                                                                                                                                                                                                                          |
| "You cannot create a default license file. License files must be added to individual repositories so the file will be included when a project is cloned, packaged, or downloaded."                                                                                                                                                                                                                 | same page                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Precedence for a file stored in more than one place: "The `.github` folder", then "The root of the repository", then "The `docs` folder"; the same order applies inside the `.github` repo; a repo's own file beats the org default.                                                                                                                                                               | same page, source lines 27-33                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Where files go: issue templates in `.github/ISSUE_TEMPLATE`, discussion forms in `.github/DISCUSSION_TEMPLATE`, FUNDING and VULNERABILITY_REPORT in `.github`; "Store all other supported files in the root of the repository, the `.github` folder, or the `docs` folder."                                                                                                                        | same page, source lines 92-96                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| CONTRIBUTING: "root, `docs`, or `.github` folder"; if several, GitHub picks `.github`, then root, then `docs`; people see a link when they open a PR or issue; a file named `CONTRIBUTING.md` also gets a Contributing tab and sidebar link.                                                                                                                                                       | [Setting guidelines for repository contributors](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/setting-guidelines-for-repository-contributors) (page summary via fetch; the location sentence also appears in the default-files source above)                                                                                                                                                                                              |
| README: "If you put your README file in your repository's hidden `.github`, root, or `docs` directory, GitHub will recognize and automatically surface your README"; with several, the order is `.github`, root, `docs`. Content beyond 500 KiB is truncated.                                                                                                                                      | [About READMEs](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-readmes); source [about-readmes.md](https://github.com/github/docs/blob/HEAD/content/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-readmes.md)                                                                                                                                             |
| SUPPORT: "you can add a SUPPORT file to your repository's root, `docs`, or `.github` folder"; people see a link when they create an issue.                                                                                                                                                                                                                                                         | [Adding support resources](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/adding-support-resources-to-your-project); source [adding-support-resources-to-your-project.md](https://github.com/github/docs/blob/HEAD/content/communities/setting-up-your-project-for-healthy-contributions/adding-support-resources-to-your-project.md) line 16                                                                                               |
| PR template: `pull_request_template.md` in root, `docs/pull_request_template.md`, or `.github/pull_request_template.md`; several templates in `PULL_REQUEST_TEMPLATE/`. Issue templates must be in `.github/ISSUE_TEMPLATE`.                                                                                                                                                                       | [source: creating-a-pull-request-template-for-your-repository.md](https://github.com/github/docs/blob/HEAD/content/communities/using-templates-to-encourage-useful-issues-and-pull-requests/creating-a-pull-request-template-for-your-repository.md) lines 27-30; [About community profiles](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/about-community-profiles-for-public-repositories)                                               |
| CODEOWNERS: "create a new file called `CODEOWNERS` in the `.github/`, root, or `docs/` directory ... If `CODEOWNERS` files exist in more than one of those locations, GitHub will search for them in that order and use the first one it finds." The most secure setup is `.github/CODEOWNERS` with an owner for the file or directory itself. Patterns follow gitignore rules and match any path. | [About code owners](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners); source [about-code-owners.md](https://github.com/github/docs/blob/HEAD/content/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners.md) lines 36, 64, 168                                                                                                           |
| SECURITY.md location on the "Adding a security policy" page: **unverified** (the rendered page fetched does not state it). The default-files page lists SECURITY.md among "all other supported files", i.e. root, `.github` or `docs`.                                                                                                                                                             | [fetched page](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/adding-a-security-policy-to-your-repository), default-files source above                                                                                                                                                                                                                                                                             |

**Community standards checklist.** The about page says only that the checklist "checks to see if a project includes
recommended community health files, such as README, CODE_OF_CONDUCT, LICENSE, or CONTRIBUTING, in a supported
location", and names no location except `.github/ISSUE_TEMPLATE` for issue templates
([About community profiles](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/about-community-profiles-for-public-repositories)).
The enumerated list is the REST response: "Get community profile metrics" returns a health percentage and a `files`
object with `code_of_conduct`, `code_of_conduct_file`, `license`, `contributing`, `readme`, `issue_template`,
`pull_request_template`; the score is "a percentage of how many of the recommended community health files are
present" ([REST: community metrics](https://docs.github.com/en/rest/metrics/community), page summary). SECURITY,
SUPPORT, GOVERNANCE, CODEOWNERS and FUNDING are not fields there. Grafana's root `GOVERNANCE.md` and
`MAINTAINERS.md` ([root listing](https://github.com/grafana/grafana)) are practice, not a GitHub feature.

**Files GitHub does not treat specially.** None of the pages above mention `ARCHITECTURE.md`,
`CODING_STANDARDS.md`, `GLOSSARY.md`, `REVIEW_RULES.md` or an ADR directory. `AGENTS.md` is not a community health
file; it is read by Copilot (section 4). **Unverified** beyond the pages cited: any other GitHub UI that surfaces
these names.

**Consequence.** A file GitHub surfaces should stay where GitHub looks. The three locations are equivalent for
discovery; `.github/` wins ties. A file GitHub does not surface can live anywhere, and the only constraint is the
reader's ability to find it from a file they do open (README, CONTRIBUTING, `AGENTS.md`).

---

## 2. What GitHub says about `docs/` and structuring repository documentation

| Fact                                                                                                                                                                                                                                                                                                     | Source                                                                                                                                                                                        |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| README purpose and content: what the project does, why it is useful, how to get started, where to get help, who maintains it.                                                                                                                                                                            | [About READMEs](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-readmes)                                            |
| "A README should only contain information necessary for developers to get started using and contributing to your project. Longer documentation is best suited for wikis."                                                                                                                                | same page, "Wikis" section                                                                                                                                                                    |
| A wiki is for "long-form content about your project, such as how to use it, how you designed it, or its core principles"; "A README file quickly tells what your project can do, while you can use a wiki to provide additional documentation."                                                          | [About wikis](https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis)                                                                                         |
| GitHub Pages can publish from "the root of the repository (`/`)" or "a `/docs` folder" of the source branch, and removing `/docs` later breaks the build. These are the only two folders the page describes.                                                                                             | [Configuring a publishing source](https://docs.github.com/en/pages/getting-started-with-github-pages/configuring-a-publishing-source-for-your-github-pages-site) (page summary)               |
| Relative links: GitHub rewrites them per branch; "Links starting with `/` will be relative to the repository root"; "Relative links are easier for users who clone your repository ... we recommend using relative links to refer to other files within your repository." Link text must be on one line. | [source: data/reusables/repositories/relative-links.md](https://github.com/github/docs/blob/HEAD/data/reusables/repositories/relative-links.md)                                               |
| GitHub's repository best practices page recommends "a README file for every repository" and says nothing about a `docs/` layout.                                                                                                                                                                         | [source: best-practices-for-repositories.md](https://github.com/github/docs/blob/HEAD/content/repositories/creating-and-managing-repositories/best-practices-for-repositories.md) lines 13-17 |
| The docs content model orders content as conceptual, referential, procedural, troubleshooting; it does not define Diataxis-style types.                                                                                                                                                                  | [About the content model](https://docs.github.com/en/contributing/style-guide-and-content-model/about-the-content-model) (page summary)                                                       |

GitHub documents no standard folder layout inside `docs/` and no guidance on `ARCHITECTURE.md` or ADRs. Note the
wiki advice does not fit a repo whose docs must be reviewed with code: wikis are separate Git repos outside pull
requests (**unverified** in the pages read; the wiki page says only that wiki files can be edited locally and that by
default only people with write access edit them).

Two cautions for us. (a) `docs/` is the one folder Pages can publish; if we ever publish from it, everything in it
becomes site content (page summary above). (b) Our `AGENTS.md` says public documentation lives in the separate
`getsphericon/getsphericon.github.io` repository, so a bare `docs/` in this repo reads as user documentation to a
newcomer; Grafana separates the two (`docs/` for user docs, `contribute/` for contributor docs; sections 6 and 7).

---

## 3. How GitHub organises its own repos

### 3.1 github/docs (the GitHub Docs repo)

- Root: `README.md`, `LICENSE`, `LICENSE-CODE`, `CHANGELOG.md`, `contributing/`, `content/`, `data/`, `src/`, no
  `AGENTS.md`, no `CONTRIBUTING.md`, no `ARCHITECTURE.md`, no `CODING_STANDARDS.md`, no `docs/` ([contents](https://github.com/github/docs)).
- `.github/`: `CODEOWNERS`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `ISSUE_TEMPLATE/`, `PULL_REQUEST_TEMPLATE.md`,
  `instructions/` (6 files), `agents/` (6), `skills/`, `workflows/`, `rulesets/`, `actions/`
  ([listing](https://github.com/github/docs/tree/HEAD/.github)). No `.github/copilot-instructions.md`.
- The in-repo `CONTRIBUTING.md` says "For comprehensive contribution guidance, please visit our official
  documentation at docs.github.com/en/contributing. This is our canonical source" and that it "provides
  repository-specific information to supplement" it ([CONTRIBUTING.md](https://github.com/github/docs/blob/HEAD/.github/CONTRIBUTING.md)).
- `contributing/` has 22 files. Reading each: **15 are one-line stubs** saying "The content in the `contributing`
  directory has been deprecated" and pointing at the published site (`content-model`, `content-style-guide`,
  `content-markup-reference`, `content-templates`, `liquid-helpers`, `redirects`, `images-and-versioning`,
  `how-to-use-labels`, `self-review`, `translations-for-writers`, `troubleshooting`, `tool-switcher`, `videos`,
  `code-annotations`, `codespace`), and 7 are live: `README.md`, `development.md`, `deployments.md`,
  `node-versions.md`, `debugging-the-docs-application.md`, `types-of-contributions.md` and
  `translations-for-translators.md`
  ([listing](https://github.com/github/docs/tree/HEAD/contributing); a script read each file for the string).
  Reading: environment and process stay in the repo; writing standards (style guide, content model, markup
  reference) moved to the published site <https://docs.github.com/en/contributing>.
- The `contributing/README.md` is an index: "Check out our contributing guide ... Here, you'll find additional
  information that might be helpful as you work on a pull request in this repo" followed by links
  ([README](https://github.com/github/docs/blob/HEAD/contributing/README.md)).
- Agent files live in `.github/instructions/*.instructions.md` with `applyTo` (e.g. `all.instructions.md` with
  `applyTo: "**"`; `code.instructions.md` with `applyTo: "src/**,.github/**,..."`). `instruction-architecture.instructions.md`
  is itself the stated policy for the split: always-on instruction files for rules that apply to every
  interaction, "roughly **150 discrete rules**" and "~6,500 tokens"; narrow, detailed guidance goes in
  `.github/agents/` ("up to 30,000 characters" each); "Do not duplicate guidance that already exists in another
  instruction file"; and "The reasoning behind these rules is written up for humans at" a separate repo
  ([instruction-architecture](https://github.com/github/docs/blob/HEAD/.github/instructions/instruction-architecture.instructions.md)).
  That is GitHub's own stated short-rules-in-the-repo, long-rationale-elsewhere split.

### 3.2 Other first-party repos

Listings via `gh api repos/<o>/<r>/contents/<path>` on 2026-10-23.

| Repo                     | Root docs files                                                                                                                                                                                               | `docs/` (or `doc/`)                                                                                                                                                                     | `.github/`                                                                                          | Source                                                                                                                                                                                            |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| github/docs              | README, LICENSE, CHANGELOG; `contributing/`                                                                                                                                                                   | none                                                                                                                                                                                    | CODEOWNERS, CODE_OF_CONDUCT, CONTRIBUTING, templates, `instructions/`, `agents/`, `skills/`         | [tree](https://github.com/github/docs), [.github](https://github.com/github/docs/tree/HEAD/.github)                                                                                               |
| github/github-mcp-server | README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, SUPPORT, LICENSE (no `AGENTS.md`)                                                                                                                            | 15 reference/how-to files (`server-configuration.md`, `testing.md`, `error-handling.md`, `policies-and-governance.md`, `installation-guides/`)                                          | CODEOWNERS, `copilot-instructions.md`, `prompts/`, `agents/`, PR and issue templates                | [tree](https://github.com/github/github-mcp-server), [docs](https://github.com/github/github-mcp-server/tree/HEAD/docs), [.github](https://github.com/github/github-mcp-server/tree/HEAD/.github) |
| github/spec-kit          | README (+3 translations), AGENTS.md, CONTRIBUTING, DEVELOPMENT.md, CODE_OF_CONDUCT, SECURITY, SUPPORT, CHANGELOG, `spec-driven.md`, `design/` (architecture notes: `cli.md`, `shared.md`, `extension.md` ...) | docfx site: `concepts/`, `guides/`, `reference/`, `install/`, `quickstart.md`, `upgrade.md`                                                                                             | CODEOWNERS, templates, `skills/`, `workflows/`, `aw/`                                               | [tree](https://github.com/github/spec-kit), [docs](https://github.com/github/spec-kit/tree/HEAD/docs), [design](https://github.com/github/spec-kit/tree/HEAD/design)                              |
| cli/cli                  | README, LICENSE, AGENTS.md                                                                                                                                                                                    | `project-layout.md`, `command-development.md`, `testing.md`, `triage.md`, `releasing.md`, `working-with-us.md`, `install_*.md`, `primer/`                                               | CONTRIBUTING, CODE-OF-CONDUCT, SECURITY, ACCESSIBILITY, CODEOWNERS, templates, `agents/`, `skills/` | [tree](https://github.com/cli/cli), [docs](https://github.com/cli/cli/tree/HEAD/docs), [.github](https://github.com/cli/cli/tree/HEAD/.github)                                                    |
| actions/runner           | README, LICENSE, **CODEOWNERS at root**                                                                                                                                                                       | `adrs/` (17 numbered ADRs + README), `design/auth.md`, `contribute/vscode.md`, `contribute.md`, `start/`, `checks/`                                                                     | `copilot-instructions.md`, templates, workflows                                                     | [tree](https://github.com/actions/runner), [docs/adrs](https://github.com/actions/runner/tree/HEAD/docs/adrs)                                                                                     |
| github/codeql            | README, CONTRIBUTING, CODE_OF_CONDUCT, **CODEOWNERS at root**                                                                                                                                                 | style guides and design patterns: `ql-style-guide.md`, `qldoc-style-guide.md`, `query-help-style-guide.md`, `query-metadata-style-guide.md`, `ql-design-patterns.md`, `change-notes.md` | `instructions/expected-files.instructions.md`, `skills/`, templates, `labeler.yml`                  | [tree](https://github.com/github/codeql), [docs](https://github.com/github/codeql/tree/HEAD/docs), [.github](https://github.com/github/codeql/tree/HEAD/.github)                                  |
| github/gh-ost            | README, LICENSE                                                                                                                                                                                               | `doc/` (note singular): `coding-ghost.md`, `local-tests.md`, `cheatsheet.md`, feature notes                                                                                             | CODEOWNERS, CONTRIBUTING, ISSUE_TEMPLATE.md, PULL_REQUEST_TEMPLATE.md                               | [tree](https://github.com/github/gh-ost), [doc](https://github.com/github/gh-ost/tree/HEAD/doc)                                                                                                   |

What the table shows:

- **None of the seven has a root `ARCHITECTURE.md` or `CODING_STANDARDS.md`.** Standards and design live under
  `docs/`/`doc/` (codeql style guides, gh-ost `coding-ghost.md`), in a `design/` folder (spec-kit), or on the
  published site (github/docs).
- **CONTRIBUTING is split**: `.github/` in github/docs, cli/cli, gh-ost; root in github-mcp-server, spec-kit and
  codeql. Both are valid under GitHub's precedence rule (section 1).
- **CODEOWNERS is split**: `.github/` (docs, mcp-server, spec-kit, cli, gh-ost) vs root (runner, codeql).
- **ADRs**: only actions/runner has a numbered ADR directory, `docs/adrs/`, with a `README.md`; the others were not
  found to have one (listings only, not a search of the whole tree).
- **Agent files**: cli/cli and spec-kit use a root `AGENTS.md`; github/docs, codeql and github-mcp-server use
  `.github/instructions/` or `.github/copilot-instructions.md`; actions/runner uses `.github/copilot-instructions.md`.
  GitHub's own repos are split, not unanimous. cli/cli's `AGENTS.md` is a gate that sends the agent to
  `.github/CONTRIBUTING.md`, "the canonical contribution policy"
  ([AGENTS.md](https://github.com/cli/cli/blob/HEAD/AGENTS.md)); spec-kit's `AGENTS.md` points to
  `design/shared.md` and `design/cli.md` before command changes
  ([AGENTS.md](https://github.com/github/spec-kit/blob/HEAD/AGENTS.md)). Both are short entry points that route to
  longer documents elsewhere.

---

## 4. GitHub's agent-facing instruction files versus documentation folders

| Fact                                                                                                                                                                                                                                                                                                                                                                                                                | Source                                                                                                                                                                                                                                                                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Types of repository instructions: repository-wide `.github/copilot-instructions.md`; path-specific `.github/instructions/**/NAME.instructions.md` with an `applyTo` frontmatter glob; agent instructions `AGENTS.md`; alternatively "a single `CLAUDE.md` or `GEMINI.md` file stored in the root".                                                                                                                  | [Add repository instructions](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions); source [add-repository-instructions.md](https://github.com/github/docs/blob/HEAD/content/copilot/how-tos/copilot-on-github/customize-copilot/add-custom-instructions/add-repository-instructions.md) lines 41-45, 141 |
| "You can create one or more `AGENTS.md` files, stored anywhere within the repository. When Copilot is working, the nearest `AGENTS.md` file in the directory tree will take precedence."                                                                                                                                                                                                                            | [source: data/reusables/copilot/custom-instructions-agents.md](https://github.com/github/docs/blob/HEAD/data/reusables/copilot/custom-instructions-agents.md)                                                                                                                                                                                              |
| `AGENTS.md` files "are different from custom agent definition files stored in `.github/agents`". Note the name collision with our `docs/agents/`.                                                                                                                                                                                                                                                                   | [source: custom-instructions.md](https://github.com/github/docs/blob/HEAD/content/copilot/concepts/prompting-and-context/custom-instructions.md) line 55                                                                                                                                                                                                   |
| Instructions "are most effective when they are short, self-contained statements"; repository instructions describe "the languages, frameworks, tools, architecture, and coding conventions used in a project".                                                                                                                                                                                                      | same source, lines 36 and 95                                                                                                                                                                                                                                                                                                                               |
| Chat precedence on github.com: personal, then path-specific, then repository-wide, then `AGENTS.md`, then organisation.                                                                                                                                                                                                                                                                                             | same source, lines 84-89                                                                                                                                                                                                                                                                                                                                   |
| Code review: "Limit any single instruction file to a maximum of about 1,000 lines. Beyond this, the quality of responses may deteriorate."; "Shorter instruction files are more likely to be fully processed"; "Begin with 10-20 specific instructions".                                                                                                                                                            | [Customize code review](https://docs.github.com/en/copilot/tutorials/customize-code-review); source [customize-code-review.md](https://github.com/github/docs/blob/HEAD/content/copilot/tutorials/customize-code-review.md) lines 58-60, 369                                                                                                               |
| Code review does not support instructions that "Follow external links": the example is `Review this code according to the standards at https://example.com/standards`, with the workaround "Copy the relevant content directly into your instruction file instead". The source gives only an external URL example; it does not say whether a relative link to a file in the same repo is followed (**unverified**). | same source, lines 351-354                                                                                                                                                                                                                                                                                                                                 |
| For code review `AGENTS.md` is described as a file "at the root of your repository" (line 239); nearest-file behaviour for nested `AGENTS.md` in code review is not stated there (**unverified**).                                                                                                                                                                                                                  | same source                                                                                                                                                                                                                                                                                                                                                |
| Copilot code review reads "repository custom instructions, agent instructions, and agent skills from the head branch (the branch with your changes), not the base branch."                                                                                                                                                                                                                                          | [source: data/reusables/copilot/code-review/custom-instructions-branch.md](https://github.com/github/docs/blob/HEAD/data/reusables/copilot/code-review/custom-instructions-branch.md)                                                                                                                                                                      |
| The cloud-agent onboarding prompt says "Instructions must be no longer than 2 pages" and lists layout and architecture facts as content to include.                                                                                                                                                                                                                                                                 | add-repository-instructions source lines 74, 102                                                                                                                                                                                                                                                                                                           |

**Does GitHub say where long standards live?** No page read says so directly. What it does say: instruction files are
short and broadly applicable; the code-review feature wants rules copied in, not linked (for external URLs); GitHub's
own docs repo keeps always-on rules small and puts detailed or rationale text in separate agent files or a separate
repo (section 3.1). The inference (ours, not GitHub's): long standards belong in a documentation folder; the
enforced subset, if Copilot review matters, is copied into `.github/instructions/` or `AGENTS.md`.

**Other agents' loading** (not GitHub, but it decides how we split):

- agents.md: "a README for agents"; "README.md files are for humans"; nested files: "Agents automatically read the
  nearest file in the directory tree, so the closest one takes precedence" ([agents.md](https://agents.md/)).
- Claude Code: reads `AGENTS.md` only when there is no `CLAUDE.md` in the working directory or above (v2.1.277 or
  later); it loads "every `AGENTS.md` ... in your working directory and the directories above it" at start and "a
  subdirectory's `AGENTS.md`, when Claude opens a file there with the Read tool"; `@path` imports inside
  `AGENTS.md` are expanded; target "under 200 lines per CLAUDE.md file"; "Claude treats them as context, not enforced
  configuration" ([Claude Code memory](https://code.claude.com/docs/en/memory), sections "AGENTS.md" and "CLAUDE.md
  files"). Our repo has no `CLAUDE.md`, so our root and nested `AGENTS.md` (`e2e/`, `internal/`, `src/`,
  `ent/schema/`) behave as documented. A root `@docs/...` import is the only mechanism found that force-loads a
  file from `docs/`; Copilot support for such imports is **unverified**.

---

## 5. ADR practice

| Source                              | Location and naming                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | URL                                                                                                                               |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Nygard, 2011, the origin            | "We will keep ADRs in the project repository under doc/arch/adr-NNN.md"; numbered "sequentially and monotonically. Numbers will not be reused"; superseded records are kept and marked; five parts: Title, Context, Decision, Status, Consequences.                                                                                                                                                                                                                                                               | [Documenting Architecture Decisions](https://www.cognitect.com/blog/2011/11/15/documenting-architecture-decisions) (page summary) |
| adr-tools (Nygard-style tooling)    | "The default directory is `doc/adr`", configurable.                                                                                                                                                                                                                                                                                                                                                                                                                                                               | [adr-tools README](https://github.com/npryce/adr-tools/blob/HEAD/README.md) lines 15-23                                           |
| MADR                                | "Copy it into `docs/decisions`"; files `nnnn-title.md`.                                                                                                                                                                                                                                                                                                                                                                                                                                                           | [adr/madr README](https://github.com/adr/madr/blob/HEAD/README.md) lines 14-24, 93-99                                             |
| adr.github.io                       | States no location and lists no templates on the home page; calls Nygard's post the one that "popularized the concept".                                                                                                                                                                                                                                                                                                                                                                                           | [adr.github.io](https://adr.github.io/) (page summary)                                                                            |
| GitHub, first party                 | actions/runner keeps numbered ADRs in `docs/adrs/` with a `README.md` ([listing](https://github.com/actions/runner/tree/HEAD/docs/adrs)). GitHub's blog post "Why write ADRs" gives no location ([post](https://github.blog/engineering/architecture-optimization/why-write-adrs/), page summary). The GitHub Well-Architected page on ADRs (`learn.github.com/well-architected/...`) rendered empty when fetched: **unverified**; no primary GitHub recommendation for `docs/decisions` or `docs/adr` was found. | listing above                                                                                                                     |
| Decision records in a separate repo | Rust RFCs ([rust-lang/rfcs](https://github.com/rust-lang/rfcs): `text/`, `0000-template.md`), Kubernetes KEPs ([kubernetes/enhancements](https://github.com/kubernetes/enhancements): `keps/`), Go proposals ([golang/proposal](https://github.com/golang/proposal): `design/`), Django DEPs ([django/deps](https://github.com/django/deps): `accepted/`, `draft/`, `rejected/`, `superseded/`). These are proposal processes, a related but different thing from ADRs.                                           | root listings                                                                                                                     |

So the ADR convention is "in the repository, in a documentation directory, numbered, never reused"; the directory
name is split four ways (`doc/arch`, `doc/adr`, `docs/adr(s)`, `docs/decisions`). Our `docs/adr/` is the most common
spelling in the sources above, not an outlier.

---

## 6. Diataxis, ARCHITECTURE.md, and the general documentation conventions

### 6.1 Diataxis

- Four forms for four needs: "tutorials, how-to guides, technical reference and explanation"; the documentation
  set is structured around them ([diataxis.fr](https://diataxis.fr/), page summary).
- Explanation: "a discursive treatment of a subject, that permits reflection. Explanation is understanding-oriented";
  its "Provide context" part names "design decisions, historical reasons, technical constraints"
  ([Explanation](https://diataxis.fr/explanation/)). That is where ADRs and an architecture overview fit; coding
  rules and the rule catalog are reference (consulted while working), not explanation.
- "Use Diátaxis as a guide, not a plan"; "Getting started with Diátaxis does not require you to think about dividing
  up your documentation into four sections"; "Diátaxis changes the structure of your documentation from the
  inside" ([How to use Diataxis](https://diataxis.fr/how-to-use-diataxis/)).
- Diataxis says nothing on repository location, and says nothing on root vs `docs/` (same page). Do not read a
  folder rule into it. (An attempt to fetch `diataxis.fr/complex-hierarchies/` returned 404; nothing is claimed from it.)
- GitHub's own content model is hierarchical (doc set, category, map topic, article) and uses
  conceptual/referential/procedural only as ordering labels, not Diataxis types
  ([content model](https://docs.github.com/en/contributing/style-guide-and-content-model/about-the-content-model),
  page summary).

### 6.2 ARCHITECTURE.md

- Origin: matklad, 2021-02-06, "ARCHITECTURE.md": add it "next to `README` and `CONTRIBUTING`"; a bird's-eye
  overview, a codemap of "coarse-grained modules and how they relate", name files and types but do not link them
  ("links go stale"), "Explicitly call-out architectural invariants", a section on cross-cutting concerns; keep it
  short, "every recurring contributor will have to read it", cover only what is "unlikely to frequently change";
  "revisit it a couple of times a year"; per-module detail belongs elsewhere
  ([ARCHITECTURE.md](https://matklad.github.io/2021/02/06/ARCHITECTURE.md.html), page summary; the post does not
  literally say "root").
- Practice in the author's own project: rust-analyzer's root has `README.md`, `CONTRIBUTING.md`, `AGENTS.md`,
  `CLAUDE.md`, `AI_POLICY.md` and **no `ARCHITECTURE.md`**; the architecture document is
  `docs/book/src/contributing/architecture.md` ("This document describes the high-level architecture of
  rust-analyzer"), next to `style.md`, `testing.md`, `setup.md`, `debugging.md`, and the root `AGENTS.md` lists
  those four as "Repository Guides"
  ([root](https://github.com/rust-analyzer/rust-analyzer),
  [contributing/](https://github.com/rust-analyzer/rust-analyzer/tree/HEAD/docs/book/src/contributing),
  [AGENTS.md](https://github.com/rust-analyzer/rust-analyzer/blob/HEAD/AGENTS.md)). Whether it was ever at the
  root is **unverified** (not checked in history).
- Tension to state plainly: the convention's wording favours a file beside README and CONTRIBUTING; the surveyed
  practice, including the originator's repo, puts it in a documentation directory and points to it from the entry
  files. Ours is a different shape too: `ARCHITECTURE.md` is 212 lines with a long "Workspace scoping" rule section,
  which is longer than matklad's "short, rarely changing" advice (the root `AGENTS.md` also carries scoping text).

### 6.3 Google and Write the Docs on docs-as-code

- "Change your documentation in the same CL as the code change"; reviewers can insist that "docstrings, header files,
  README.md files, and any other docs get updated alongside the CL"; "A good README.md orients the new user to the
  directory and points to more detailed explanation and user guides"; "Do not write your own guide to a common
  Google technology or process. Link to it instead"; "Dead docs are bad ... Default to delete or leave behind if
  migrating"; "A small set of fresh and accurate docs is better than a large assembly of 'documentation' in
  various states of disrepair"; teams that keep docs elsewhere should "clearly point to those locations from your
  project directory" ([Documentation best practices](https://google.github.io/styleguide/docguide/best_practices.html),
  page summary).
- "Docs thrive when they're treated like tests"; "Engineers should be able to interact with documentation using
  the same tools they use to read and write code"; prefer plain, human-readable source
  ([Google documentation philosophy](https://google.github.io/styleguide/docguide/philosophy.html), page summary).
  The page does not mention repositories or location.
- Write the Docs: docs as code means the same tools and workflows as code, listing issue trackers, version control,
  plain-text markup, code review and automated tests ([Docs as code](https://www.writethedocs.org/guide/docs-as-code/),
  page summary). It does not say docs must share a repository with code (same page).
- Directory-bound rules next to code, cross-cutting rules central: Google's "package-level README.md"; PostgreSQL
  keeps `README`, `README-SSI`, `README.barrier` inside `src/backend/storage/lmgr/`
  ([listing](https://github.com/postgres/postgres/tree/HEAD/src/backend/storage/lmgr)); Grafana's root `AGENTS.md`
  begins "Directory-scoped agent files exist for specialized areas" and lists `docs/AGENTS.md`, an alerting
  `AGENTS.md` and a storage `AGENTS.md` ([AGENTS.md](https://github.com/grafana/grafana/blob/HEAD/AGENTS.md)). Our
  nested `AGENTS.md` files follow the same pattern. Neither Google nor Write the Docs states a rule for choosing
  between "next to code" and "central"; that choice is ours (**no primary rule found**).
- Single source of truth: Sentry's `AGENTS.md` opens "AGENTS.md files are the source of truth for AI agent
  instructions ... Do not add to CLAUDE.md or Cursor rules" ([AGENTS.md](https://github.com/getsentry/sentry/blob/HEAD/AGENTS.md));
  Grafana's `CLAUDE.md` is the single line `@AGENTS.md` ([CLAUDE.md](https://github.com/grafana/grafana/blob/HEAD/CLAUDE.md));
  Zephyr's `AGENTS.md` is a digest where "the documentation wins" (see the companion note).

---

## 7. Layouts of mature open-source projects

| Project                                 | Root                                                                                                                                                                                                                   | Contributor rules and workflow                                                                                                       | Coding standards                                                                                                            | Architecture                                                            | Decisions                                                 | Source                                                                                                                                                                                                                                                                                                           |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kubernetes                              | `CONTRIBUTING.md` (a pointer: "check out the Contributor's Guide"), `AGENTS.md`, `SUPPORT.md`, `OWNERS`, `docs/` holds only `OWNERS` and `.gitignore`                                                                  | separate repo `kubernetes/community`, `contributors/guide/` (`contributing.md`, `pull-requests.md`, `review-guidelines.md`)          | `contributors/guide/coding-conventions.md`, `style-guide.md` in that repo                                                   | `contributors/devel/` and `sig-architecture/` in that repo              | KEPs in `kubernetes/enhancements`                         | [k8s root](https://github.com/kubernetes/kubernetes), [CONTRIBUTING](https://github.com/kubernetes/kubernetes/blob/HEAD/CONTRIBUTING.md), [community guide](https://github.com/kubernetes/community/tree/HEAD/contributors/guide), [devel](https://github.com/kubernetes/community/tree/HEAD/contributors/devel) |
| Rust (`rust-lang/rust`)                 | `CONTRIBUTING.md` ("Documentation for contributing to the compiler or tooling is located in the Guide to Rustc Development"), `AGENTS.md`, `README.md`, `INSTALL.md`                                                   | `rustc-dev-guide` (a book, `src/` is in `rust-lang/rustc-dev-guide`, mirrored under `src/doc/rustc-dev-guide`)                       | `rustc-dev-guide/src/conventions.md`, `src/doc/style-guide`                                                                 | rustc-dev-guide chapters (`overview.md`, `hir.md`, ...)                 | `rust-lang/rfcs`                                          | [rust root](https://github.com/rust-lang/rust), [CONTRIBUTING](https://github.com/rust-lang/rust/blob/HEAD/CONTRIBUTING.md), [guide](https://github.com/rust-lang/rustc-dev-guide/tree/HEAD/src)                                                                                                                 |
| rust-analyzer                           | `CONTRIBUTING.md`, `AGENTS.md`, `CLAUDE.md`, `AI_POLICY.md`, `README.md`                                                                                                                                               | `docs/book/src/contributing/README.md`                                                                                               | `docs/book/src/contributing/style.md`                                                                                       | `docs/book/src/contributing/architecture.md`                            | none found in root or docs (**unverified**: not searched) | see 6.2                                                                                                                                                                                                                                                                                                          |
| Go (`golang/go`)                        | `CONTRIBUTING.md` ("Please read the Contribution Guidelines" on golang.org/doc/contribute.html), `README.md`, `SECURITY.md`; `doc/` holds the spec, memory model, asm                                                  | published contribution guide (site, separate repo)                                                                                   | published (site); **unverified** which page                                                                                 | published                                                               | `golang/proposal`                                         | [go root](https://github.com/golang/go), [CONTRIBUTING](https://github.com/golang/go/blob/HEAD/CONTRIBUTING.md), [doc](https://github.com/golang/go/tree/HEAD/doc)                                                                                                                                               |
| Django                                  | `CONTRIBUTING.rst` (points to `docs/internals/contributing/`), `README.rst`; `.github/` has `CODE_OF_CONDUCT.md`, `SECURITY.md`, `copilot-instructions.md`, PR template; no root `AGENTS.md`                           | `docs/internals/contributing/` (`new-contributors`, `committing-code`, `triaging-tickets`, `writing-documentation`, `writing-code/`) | `docs/internals/contributing/writing-code/coding-style.txt`, `javascript.txt`, `unit-tests.txt`                             | in the published manual (**unverified** for a single architecture page) | `django/deps`                                             | [django root](https://github.com/django/django), [CONTRIBUTING.rst](https://github.com/django/django/blob/HEAD/CONTRIBUTING.rst), [contributing/](https://github.com/django/django/tree/HEAD/docs/internals/contributing)                                                                                        |
| Grafana                                 | `AGENTS.md`, `CLAUDE.md` (`@AGENTS.md`), `AI_POLICY.md`, `CONTRIBUTING.md`, `GOVERNANCE.md`, `MAINTAINERS.md`, `ROADMAP.md`, `SUPPORT.md`, `WORKFLOW.md`, `docs/` (user docs, with `docs/AGENTS.md`) and `contribute/` | `contribute/` (`create-pull-request.md`, `merge-pull-request.md`, `developer-guide.md`, `deprecation-policy.md`)                     | `contribute/style-guides/` (`frontend.md`, `testing.md`, `styling.md`, ...), `contribute/backend/style-guide.md`            | `contribute/architecture/`                                              | not found (**unverified**)                                | [root](https://github.com/grafana/grafana), [contribute/](https://github.com/grafana/grafana/tree/HEAD/contribute), [style-guides](https://github.com/grafana/grafana/tree/HEAD/contribute/style-guides)                                                                                                         |
| GitLab (`gitlabhq/gitlabhq` mirror)     | `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md` (CLA/DCO), `PHILOSOPHY.md`, `MAINTENANCE.md`, `.ai/` (instruction files by task)                                                                                           | `doc/development/contributing/`, `doc/development/code_review.md`, `definition_of_done.md`                                           | `doc/development/*styleguide*`, `doc/development/contributing/style_guides.md`, `doc/development/documentation/styleguide/` | `doc/development/architecture.md`                                       | not found (**unverified**)                                | [root](https://github.com/gitlabhq/gitlabhq), [doc/development](https://github.com/gitlabhq/gitlabhq/tree/HEAD/doc/development), [AGENTS.md](https://github.com/gitlabhq/gitlabhq/blob/HEAD/AGENTS.md)                                                                                                           |
| PostgreSQL (`postgres/postgres` mirror) | `README.md` (links to the online manual), `COPYRIGHT`, `HISTORY`; `.github/` has `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`                                                                                | the manual (`doc/src`) and wiki; **unverified** which pages                                                                          | in the manual (**unverified**)                                                                                              | READMEs beside code, e.g. `src/backend/storage/lmgr/README`             | not found                                                 | [root](https://github.com/postgres/postgres), [.github](https://github.com/postgres/postgres/tree/HEAD/.github)                                                                                                                                                                                                  |
| Sentry (`getsentry/sentry`)             | `AGENTS.md` (source of truth for agents), `CLAUDE.md`, `README.md`, `CHANGES`; no `docs/`                                                                                                                              | separate repo `getsentry/develop` (a Gatsby site); the exact develop-docs path was not located (**unverified**)                      | **unverified**                                                                                                              | **unverified**                                                          | **unverified**                                            | [sentry root](https://github.com/getsentry/sentry), [develop](https://github.com/getsentry/develop)                                                                                                                                                                                                              |

Observations:

- **Root = entry points + governance, not the manual.** Every project's root holds `README`, a `CONTRIBUTING`
  (often a pointer), licence, and in the newer ones an `AGENTS.md`. Grafana's root is the widest (`GOVERNANCE`,
  `MAINTAINERS`, `ROADMAP`, `WORKFLOW`) but still keeps standards in `contribute/`.
- **Contributor documentation is a named directory or a separate guide.** Names differ: `contribute/` (Grafana),
  `doc/development/` (GitLab), `docs/internals/contributing/` (Django), `docs/book/src/contributing/`
  (rust-analyzer), a separate repo (Kubernetes, Rust, GitHub docs' published guide).
- **Standards are never at the root.** Django, Kubernetes, Grafana, GitLab, rust-analyzer, Rust all put the coding
  style in a style-guide file under the contributor directory.
- **Split on in-repo vs separate guide**: Kubernetes, Rust, Go, GitHub Docs moved the long guide out of the code
  repository; Django, Grafana, GitLab, rust-analyzer, GitHub CLI keep it in. A single small repo like ours is in the
  second group.
- **A numbered review-rules catalog separate from the style guide was not observed** in any surveyed repo; do not
  read this as "nobody does it". The nearest separate review documents are Kubernetes
  [`review-guidelines.md`](https://github.com/kubernetes/community/blob/HEAD/contributors/guide/review-guidelines.md),
  GitLab `doc/development/code_review.md` and `.ai/code-review.md`, and Grafana's "Code Review Rules" section inside
  `AGENTS.md` ([AGENTS.md](https://github.com/grafana/grafana/blob/HEAD/AGENTS.md)). Google's engineering practices
  list what a reviewer looks for (Design, Functionality, Complexity, Tests, Naming, Comments, Style, Consistency,
  Documentation) as a document apart from the language style guides, and say "the style guide is the absolute
  authority" ([What to look for](https://google.github.io/eng-practices/review/reviewer/looking-for.html), page
  summary). That is the closest primary support for keeping a review catalog beside, not inside, the style guide.

---

## 8. Synthesis for our documents

### 8.1 What each convention implies

| Document                          | GitHub's rule                                                                                                             | General practice                                                                                                                                                                                                                       | Verdict                                                                                                                                                                                                                                                         |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `README.md`                       | Surfaced from `.github/`, root or `docs/` in that order; keep it to getting started and contributing, long text elsewhere | Root, everywhere                                                                                                                                                                                                                       | **Root.** Strong.                                                                                                                                                                                                                                               |
| `CONTRIBUTING.md`                 | Same three locations, `.github` first; link shown on PR and issue creation, Contributing tab if named `CONTRIBUTING.md`   | Split: `.github/` (github/docs, cli, gh-ost) vs root (mcp-server, spec-kit, codeql, Rust, Go, k8s, rust-analyzer, Grafana, GitLab); often a pointer                                                                                    | **Root or `.github/`; both fully supported.** Root keeps it visible to people and agents browsing the tree; `.github/` is GitHub's own most common tie-break. 221 lines is already large: github/docs and Kubernetes keep it short and point to a fuller guide. |
| `AGENTS.md` (root + nested)       | Not a community file; Copilot reads it "anywhere ... nearest wins"                                                        | Root `AGENTS.md` in Kubernetes, Rust, rust-analyzer, Grafana, GitLab, Sentry, cli/cli, spec-kit; nested in Grafana                                                                                                                     | **Root, plus nested for directory-bound rules (already our shape).** Keep it a short router: GitHub's 150-rule and 200-line (Claude) budgets, and Copilot review's "about 1,000 lines" cap, apply to what it contains.                                          |
| `CODING_STANDARDS.md`             | Not special                                                                                                               | Never at the root in any survey; `docs/`, `contribute/style-guides/`, `doc/development/`, `writing-code/coding-style`                                                                                                                  | **Move under `docs/`.** Strong.                                                                                                                                                                                                                                 |
| Rules catalog (`REVIEW_RULES.md`) | Not special                                                                                                               | No surveyed repo has a numbered catalog; Google keeps review guidance separate from the style guide                                                                                                                                    | **Under `docs/`, next to the standards** (a sibling file, not merged), since a reviewer consults it as reference.                                                                                                                                               |
| `ARCHITECTURE.md`                 | Not special                                                                                                               | matklad: next to README and CONTRIBUTING; practice: in the docs tree (rust-analyzer, GitLab `architecture.md`, Grafana `contribute/architecture/`, spec-kit `design/`)                                                                 | **Split decision (section 8.3).** The scoping rules are reference, not a codemap, and belong under `docs/`.                                                                                                                                                     |
| `GLOSSARY.md`                     | Not special                                                                                                               | No surveyed repo has one; ubiquitous-language files are not covered by the sources read. Our vendored skills default to `GLOSSARY.md` and `docs/adr/` at the repo root (single-context) and read the path from `docs/agents/domain.md` | **Root is cheapest** (929 lines, read by skills, `AGENTS.md` and the domain skills); moving is supported by the skills' own config but touches every prompt. No primary evidence either way.                                                                    |
| ADRs (`docs/adr/`)                | Not special                                                                                                               | `docs/adr(s)`, `docs/decisions`, `doc/arch`; in-repo, numbered                                                                                                                                                                         | **Keep `docs/adr/`.** Strong.                                                                                                                                                                                                                                   |
| `docs/research/`                  | Not special                                                                                                               | Not in any surveyed repo (**unverified**; not searched)                                                                                                                                                                                | Keep under `docs/`. Our own convention.                                                                                                                                                                                                                         |
| `docs/design/`                    | Not special                                                                                                               | spec-kit `design/` at root; GitLab and Grafana under their contributor directory                                                                                                                                                       | Keep under `docs/`.                                                                                                                                                                                                                                             |
| Tooling docs (`docs/agents/`)     | Not special                                                                                                               | Contributor how-to belongs with contributor docs: github/docs `contributing/development.md`, cli `docs/`, GitLab `doc/development/`                                                                                                    | **Keep under `docs/`, but rename**: GitHub reserves `.github/agents` for custom agent definitions, so `docs/agents/` reads as that; the files are issue tracker, triage labels, worktrees, dev environment, codegen: process and tooling.                       |

### 8.2 Candidate layouts ranked by evidence

1. **Thin root, standards/architecture/rules under `docs/` with subfolders** (recommended). Supported by Kubernetes,
   Rust, rust-analyzer, Django, Grafana, GitLab and the first-party repos (codeql, cli, gh-ost, spec-kit, github
   docs, mcp-server): the only variance is the folder name. Root: `README.md`, `CONTRIBUTING.md`, `AGENTS.md`,
   `GLOSSARY.md`, `CODING_STANDARDS.md` (the vendored `code-review` and `retro` skills look for it at the root, so it stays there), `LICENSE`, plus conduct and security files. `docs/` (all engineering):
   - `docs/standards/` (or `docs/contributing/`): `review-rules.md`
   - `docs/architecture.md` (or `docs/architecture/` with the scoping rule as its own page)
   - `docs/adr/`, `docs/design/`, `docs/research/`
   - `docs/tooling/` (the current `docs/agents/`: codegen, dev environment, worktrees, issue tracker, triage labels)
2. **Root-heavy (everything stays at the root).** Evidence: matklad's wording for `ARCHITECTURE.md`; spec-kit's
   root `DEVELOPMENT.md`, `spec-driven.md`, `design/`; Grafana's wide root, which holds governance not standards.
   No surveyed project keeps standards or rules at the root; the root grows with each new document and GitHub's
   Contributing tab or sidebar cannot surface any of them (only CONTRIBUTING).
3. **Move community files to `.github/`** (CONTRIBUTING, SECURITY, templates, CODEOWNERS). Supported by GitHub's own
   precedence order and by github/docs, cli, gh-ost, but it does not matter for engineering documents: no surveyed
   repo puts standards or architecture in `.github/`; there it holds community files, workflows and Copilot files.
4. **Separate guide repo or site** (Kubernetes, Rust, Go, GitHub Docs' published contributor guide). Strong for very
   large projects, but it moves documents out of PR review next to the code; for a single repo with a docs site
   already in another repo this is possible later and not evidenced as needed now.
5. **A wiki** (GitHub's literal advice for long documentation). Weakest for us: it is outside pull requests, outside
   the tree agents clone, and no surveyed engineering project uses it (the wiki page read says nothing on review).

### 8.3 Where practice is split, plainly

- **CONTRIBUTING root vs `.github/`.** Split; both valid. GitHub's order only decides which one wins when both exist.
- **`ARCHITECTURE.md` at the root vs in the docs tree.** The advice (matklad) and the practice (his own project,
  GitLab, Grafana, spec-kit) disagree; with no root file in any survey, a root `ARCHITECTURE.md` is the less
  supported choice. A compromise with evidence behind it: a short root-level pointer or an `AGENTS.md` link to
  `docs/architecture.md`, as rust-analyzer's `AGENTS.md` does.
- **Docs-folder name.** `docs/` is the common and Pages-compatible name but collides with the meaning "user
  documentation" (Grafana avoids it with `contribute/`; GitLab with `doc/development/`; Django with
  `docs/internals/`). For us, `docs/` is acceptable because the public documentation is in another repo, and our
  `docs/` already holds ADRs, research and design notes.
- **In-repo vs separate guide.** Split by project size; ours is in the in-repo group.
- **Where "directory-bound" rules live.** No primary source states the rule; the surveyed practice (Grafana,
  Postgres, Google's package READMEs) puts rules that apply to one directory next to that code and the rest in
  the central docs.
- **`GLOSSARY.md`, `docs/research/`, a numbered rules catalog.** No surveyed repo has one; there is no primary
  evidence for a location.

### 8.4 CODEOWNERS and agents' loading: do they favour a location?

- **CODEOWNERS** matches by path pattern and "follows most of the same rules used in gitignore files"; the three
  supported locations are `.github/`, root, `docs/`, in that order, and GitHub's own security note recommends
  `.github/CODEOWNERS` with the file or `.github/` itself owned by the repository owner
  ([About code owners](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)).
  Document location does not constrain it, so it favours no location for the documents. Grouping them does help:
  one `/docs/standards/` (or `/docs/contributing/`) line and a line for `AGENTS.md` and `**/AGENTS.md` is easier
  than listing five root files. Our worktree has no CODEOWNERS file at all (`.github/` has `actions`, `codeql`,
  `workflows`, `dependabot.yml`, `pull_request_template.md`).
- **Why the agent files and rules deserve an owner.** Copilot code review reads instructions "from the head branch ...
  not the base branch" (source in section 4), so a pull request that edits `AGENTS.md` or an instruction file
  changes the instructions used to review that same pull request. An owner on those paths is a direct response to
  that; the companion note covers rulesets and required reviews.
- **Agent loading** favours short, route-and-gate entry files at root plus nested files for directories, not any
  `docs/` location: nothing auto-loads `docs/` (sections 4 and 8.1); a document in `docs/` is read when an
  instruction file points to it (as rust-analyzer's and cli's `AGENTS.md` do), or, in Claude Code, when a root
  `AGENTS.md` imports it with `@path` (at the cost of loading it every session).
  For Copilot code review, an enforced rule should sit in the instruction file rather than behind a link; the
  source shows this only for external URLs, so test the relative-link case before relying on it.
- **Cost of moving files here.** Our repo has 19 Go files, `mise/tasks.toml`, the analyzers' error messages and
  `CODING_STANDARDS.md`/`REVIEW_RULES.md`/`ARCHITECTURE.md` cross-references, five ADRs, `docs/agents/domain.md` and
  the nested `AGENTS.md` files that name these files (counted with `rg` for the filenames, excluding
  `docs/research/`). A link checker (lychee, per the recent commit) can catch broken Markdown links; Go
  comments and analyzer messages need a manual sweep. The vendored skills read `GLOSSARY.md` and `docs/adr/` at
  the root by default (single-context), which is a reason not to move those two.

### 8.5 Recommendation

Adopt layout 1 with these concrete moves (none is dictated by GitHub; each is the best-supported choice in the
survey):

- Keep at root: `README.md`, `CONTRIBUTING.md` (trim and point to `docs/`), `AGENTS.md` and nested `AGENTS.md`,
  `GLOSSARY.md`, licence, conduct and security files.
- Move under `docs/`: `CODING_STANDARDS.md` and `REVIEW_RULES.md` to a standards folder as siblings;
  `ARCHITECTURE.md` to `docs/architecture.md` (with `AGENTS.md` and `CONTRIBUTING.md` linking to it); rename
  `docs/agents/` to `docs/tooling/` to avoid the `.github/agents` meaning.
- Keep `docs/adr/`, `docs/design/`, `docs/research/` as they are.
- Add `.github/CODEOWNERS` covering the standards folder, `AGENTS.md`, `**/AGENTS.md` and `.github/` itself.
- Trade-offs: a thin root is the dominant practice and keeps GitHub's UI surfacing intact, at the cost of one
  hop (a link) from the entry files to the standards, and a one-time reference sweep. Keeping them at the root
  saves the sweep but has no support in the survey and lets the root grow. A root `ARCHITECTURE.md` follows the
  article but not its author's own repo.

---

## 9. Items marked unverified, and gaps

- docs.github.com pages summarised by `WebFetch` (setting guidelines, community profile, REST community metrics,
  Pages source, content model, Claude Code memory, agents.md, Google and Write the Docs pages, Diataxis, Nygard,
  adr.github.io, matklad): the quotes are from a model's summary of the page, not raw source. The ones that carry
  weight (default health files, README, CODEOWNERS, SUPPORT, PR template, relative links, Copilot instructions)
  were re-read in the raw `github/docs` source and cited by path.
- SECURITY.md location on the "adding a security policy" page: not stated on the page fetched.
- GitHub Well-Architected ADR guidance: the page rendered empty; no GitHub-recommended ADR directory found.
- Whether a relative in-repo link in an instruction file is followed by Copilot code review; whether nested
  `AGENTS.md` is honoured by Copilot code review; whether Copilot expands `@path` imports.
- Wiki behaviour in pull requests: not confirmed on the pages read.
- rust-analyzer: whether `ARCHITECTURE.md` was ever at the root; decision records: not searched.
- Go's, Django's and PostgreSQL's separate guides and manual pages (coding style, architecture): only the pointers
  in the repos were read, not the published guides.
- Sentry's develop docs path (`getsentry/develop` listing did not show a `develop-docs/` directory).
- Whether `docs/research/`, `GLOSSARY.md`-style ubiquitous-language files or numbered review-rule catalogs exist
  in other projects: not searched exhaustively; absence in the surveyed set is not a census.
- The first-party repo list and the mature-project list are samples chosen for relevance (an open-core Go and React
  repo with outside contributors), not a census. File listings reflect the default branch on 2026-10-23.
- ARCHITECTURE.md wording ("next to README and CONTRIBUTING") is from a page summary; the original post should be
  re-read before quoting it in a decision record.
