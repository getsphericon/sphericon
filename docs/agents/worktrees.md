# Worktrees

Worktrees live where Claude Code puts them by default: `.claude/worktrees/<name>` inside the
primary checkout (gitignored; Vite ignores it).

- Create them with the `EnterWorktree` tool or `claude --worktree`.
- Every change is made in a worktree, however small (a doc line, a typo, a config tweak). Nothing is
  edited or committed in the primary checkout: it only tracks `origin/main` (`git fetch origin && git
  rebase origin/main`) and is where the dev stack, Postgres and `worktrees:rm` run.
- Every worktree works on its own branch, which is what `EnterWorktree` and `claude --worktree`
  create (`worktree-<name>`); git allows a branch in one worktree only, and the primary checkout
  holds `main`. Create no branch in the primary checkout, which is shared and stays on `main`.
  `mise run land` refuses `main` and a detached `HEAD`, pushes the branch and opens the pull request;
  nothing is pushed to `main` (the repository ruleset requires a pull request).
- Never use `git worktree add ../sphericon-<name>` or any other sibling directory.
- The directory name feeds the checkout's database and origin (`sphericon_<dir>`,
  `<dir>.sphericon.localhost`), so keep `<name>` short and unique.
- Cleanup after a merge is described under "Conventions" in `AGENTS.md`. A session standing in the
  worktree leaves it with `ExitWorktree` (`keep`), then runs `mise run worktrees:rm <path>` from the
  primary checkout; `ExitWorktree` with `remove` and a plain `git worktree remove` leave the
  worktree's databases behind. `--force` removes a worktree with uncommitted changes.
- `worktrees:rm` and `worktrees:prune` delete the local branch once it is merged. A pull request is
  squashed, so its commits are not in `origin/main` under their own SHAs: the branch counts as merged
  when its remote branch is gone after `git fetch --prune`, which GitHub does on merge (the repository
  setting "Automatically delete head branches"). Until that setting is on, a squashed branch is kept.
