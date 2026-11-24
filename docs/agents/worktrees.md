# Worktrees

Worktrees live in `.worktrees/<name>` inside the primary checkout. Worktrees are a git concept that
several tools use, so they get one layout, ignored by `.gitignore` for everyone (and by Vite, the
linters and the link check), rather than the one Claude Code picks by default
(`.claude/worktrees/<name>`). The `WorktreeCreate` and `WorktreeRemove` hooks in
`.claude/settings.json` (`.claude/hooks/worktree.sh`) put Claude Code's worktrees there too.

- Create them with the `EnterWorktree` tool or `claude --worktree`: the hook makes
  `.worktrees/<name>` on the branch `worktree-<name>` from `origin/main`. By hand: `git fetch origin
  && git worktree add --no-track -b worktree-<name> .worktrees/<name> origin/main`.
- A worktree made before under `.claude/worktrees/<name>` moves with `git worktree move
  .claude/worktrees/<name> .worktrees/<name>`; its directory name, and with it its database and
  origin, stay the same.
- Every change is made in a worktree under `.worktrees/<name>`, on a branch of its own; the primary checkout stays on a clean `main` and no branch is created there. The one exception is a small visual fix that has to be seen at once in the app running from the primary checkout: it is made there, committed on `main` and landed with `git fetch origin && git rebase origin/main && git push origin main`. A doc line or a typo is not such a fix.
  Otherwise the primary checkout only tracks `origin/main` (`git fetch origin && git rebase
  origin/main`) and is where the dev stack, Postgres and `worktrees:rm` run.
- Every worktree works on its own branch, which is what `EnterWorktree` and `claude --worktree`
  create (`worktree-<name>`); git allows a branch in one worktree only, and the primary checkout
  holds `main`. Create no branch in the primary checkout, which is shared and stays on `main`.
  `mise run land` refuses `main` and a detached `HEAD`, pushes the branch and opens the pull request.
  A maintainer may instead push the worktree's commits straight to `main` (`git fetch origin && git
  rebase origin/main && git push origin HEAD:main`): the repository ruleset lets the admin role
  bypass the pull request. Nothing else is pushed from the primary checkout, which stays on a clean `main`.
- Never use `git worktree add ../sphericon-<name>` or any other sibling directory.
- The directory name feeds the checkout's database and origin (`sphericon_<dir>`,
  `<dir>.sphericon.localhost`), so keep `<name>` short and unique.
- Cleanup after a merge is described under "Conventions" in `AGENTS.md`. A session standing in the
  worktree leaves it with `ExitWorktree`: `remove` runs `mise run worktrees:rm <path>` through the
  `WorktreeRemove` hook, and `keep` leaves it for `mise run worktrees:rm <path>` from the primary
  checkout. A plain `git worktree remove` leaves the worktree's databases behind. `--force` removes
  a worktree with uncommitted changes.
- `worktrees:rm` and `worktrees:prune` delete the local branch once it is merged. A pull request is
  squashed, so its commits are not in `origin/main` under their own SHAs: the branch counts as merged
  when its remote branch is gone after `git fetch --prune`, which GitHub does on merge (the repository
  setting "Automatically delete head branches"). Until that setting is on, a squashed branch is kept.
