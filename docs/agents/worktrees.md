# Worktrees

Worktrees live where Claude Code puts them by default: `.claude/worktrees/<name>` inside the
primary checkout (gitignored; Vite ignores it).

- Create them with the `EnterWorktree` tool or `claude --worktree`.
- Never create a branch in the primary checkout (`git switch -c`, `git checkout -b`): it is shared
  and stays on `main`. Work that needs a branch happens in a worktree, and only when it is needed.
- Never use `git worktree add ../sphericon-<name>` or any other sibling directory.
- The directory name feeds the checkout's database and origin (`sphericon_<dir>`,
  `<dir>.sphericon.localhost`), so keep `<name>` short and unique.
- Cleanup after a merge is described under "Conventions" in `AGENTS.md`.
