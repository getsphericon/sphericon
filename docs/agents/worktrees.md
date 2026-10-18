# Worktrees

Worktrees live where Claude Code puts them by default: `.claude/worktrees/<name>` inside the
primary checkout (gitignored; Vite ignores it).

- Create them with the `EnterWorktree` tool or `claude --worktree`.
- A trivial change (a doc line, a typo, a config tweak) is committed straight to `main` in the
  primary checkout. Anything larger is done in a worktree.
- Never create a branch (`git switch -c`, `git checkout -b`, `git branch`), in the primary checkout
  or in a worktree: the primary checkout is shared and stays on `main`.
- Never use `git worktree add ../sphericon-<name>` or any other sibling directory.
- The directory name feeds the checkout's database and origin (`sphericon_<dir>`,
  `<dir>.sphericon.localhost`), so keep `<name>` short and unique.
- Cleanup after a merge is described under "Conventions" in `AGENTS.md`.
