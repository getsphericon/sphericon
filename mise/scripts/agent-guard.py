#!/usr/bin/env python3
"""PreToolUse hook for the Bash tool of Claude Code (.claude/settings.json).

It refuses the two commands that skip the checks the flow depends on, and says what to run instead:

  - any git command with --no-verify, which skips the commit hooks;
  - `git push ... HEAD:main`, the landing push of a worktree: `mise run land` rebases, runs the
    checks and the tests and only then pushes.

Exit status 2 refuses the call and shows stderr to the agent; any other exit lets it run.
"""
import json
import re
import sys

NO_VERIFY = re.compile(r"\bgit\b[^|;&\n]*--no-verify\b")
LANDING_PUSH = re.compile(r"\bgit\s+push\b[^|;&\n]*\bHEAD:main\b")


HEREDOC = re.compile(r"<<-?\s*['\"]?\w+")
QUOTED = re.compile(r"\"[^\"]*\"|'[^']*'")


def code_of(command: str) -> str:
    """The command without what it only says: a heredoc body and quoted text (a commit message)."""
    return QUOTED.sub('""', HEREDOC.split(command, maxsplit=1)[0])


def refusal(command: str) -> str | None:
    command = code_of(command)
    if NO_VERIFY.search(command):
        return "git --no-verify skips the hooks that keep the checks honest. Fix what the hook reports instead."
    if LANDING_PUSH.search(command):
        return (
            "Land with `mise run land -- [--skip-test] [<issue>...]`: it rebases onto origin/main, runs "
            "`mise run check` and `mise run test`, pushes and checks the ticket. A bare push of HEAD to main "
            "skips all of that."
        )
    return None


def main() -> int:
    try:
        event = json.load(sys.stdin)
    except json.JSONDecodeError:
        return 0
    command = event.get("tool_input", {}).get("command", "")
    message = refusal(command) if isinstance(command, str) else None
    if message:
        print(message, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
