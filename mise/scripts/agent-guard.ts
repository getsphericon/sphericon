// PreToolUse hook for the Bash tool of Claude Code (.claude/settings.json).
//
// It refuses the two commands that skip the checks the flow depends on, and says what to run instead:
//
//   - any git command with --no-verify, which skips the commit hooks;
//   - `git push ... HEAD:main`, the landing push of a worktree: `mise run land` rebases, runs the
//     checks and the tests and only then pushes.
//
// Exit status 2 refuses the call and shows stderr to the agent; any other exit lets it run.
import { readFileSync } from 'node:fs'

const noVerify = /\bgit\b[^|;&\n]*--no-verify\b/
const landingPush = /\bgit\s+push\b[^|;&\n]*\bHEAD:main\b/
const heredoc = /<<-?\s*['"]?\w+/
const quoted = /"[^"]*"|'[^']*'/g

// What the command does, without what it only says: a heredoc body and quoted text (a commit message).
function codeOf(command: string): string {
  const [head = ''] = command.split(heredoc, 1)
  return head.replace(quoted, '""')
}

function refusal(command: string): string | undefined {
  const code = codeOf(command)
  if (noVerify.test(code)) {
    return 'git --no-verify skips the hooks that keep the checks honest. Fix what the hook reports instead.'
  }
  if (landingPush.test(code)) {
    return (
      'Land with `mise run land -- [--skip-test] [<issue>...]`: it rebases onto origin/main, runs ' +
      '`mise run check` and `mise run test`, pushes and checks the ticket. A bare push of HEAD to main ' +
      'skips all of that.'
    )
  }
  return undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function commandOf(input: string): string {
  try {
    const event: unknown = JSON.parse(input)
    const toolInput = isRecord(event) ? event.tool_input : undefined
    const command = isRecord(toolInput) ? toolInput.command : undefined
    return typeof command === 'string' ? command : ''
  } catch {
    return ''
  }
}

const message = refusal(commandOf(readFileSync(0, 'utf8')))
if (message !== undefined) {
  process.stderr.write(`${message}\n`)
  process.exit(2)
}
