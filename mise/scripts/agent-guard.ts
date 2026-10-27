// PreToolUse hook for the Bash tool of Claude Code (.claude/settings.json).
//
// It refuses the commands that skip the checks the flow depends on, and says what to run instead:
//
//   - any git command with --no-verify, which skips the commit hooks;
//   - any `git push` that names main (`HEAD:main`, `origin main`, `:main`): work reaches main through
//     a pull request, which `mise run land` rebases, checks, pushes and opens;
//   - `gh pr merge --admin`, which merges past the required checks.
//
// Exit status 2 refuses the call and shows stderr to the agent; any other exit lets it run.
import { readFileSync } from 'node:fs'

const noVerify = /\bgit\b[^|;&\n]*--no-verify\b/
const push = /\bgit\s+push\b([^|;&\n]*)/g
const adminMerge = /\bgh\s+pr\s+merge\b[^|;&\n]*--admin\b/
const heredoc = /<<-?\s*['"]?\w+/
const quoted = /"[^"]*"|'[^']*'/g

// What the command does, without what it only says: a heredoc body and quoted text (a commit message).
function codeOf(command: string): string {
  const [head = ''] = command.split(heredoc, 1)
  return head.replace(quoted, '""')
}

// A push names main as a bare argument (`origin main`) or as the destination of a refspec (`HEAD:main`).
function pushesToMain(code: string): boolean {
  return [...code.matchAll(push)].some(([, rest = '']) =>
    rest.split(/\s+/).some((arg) => arg === 'main' || /:(refs\/heads\/)?main$/.test(arg)),
  )
}

function refusal(command: string): string | undefined {
  const code = codeOf(command)
  if (noVerify.test(code)) {
    return 'git --no-verify skips the hooks that keep the checks honest. Fix what the hook reports instead.'
  }
  if (pushesToMain(code)) {
    return (
      'Nothing is pushed to main. Open a pull request with `mise run land -- --door <one-way|two-way> ' +
      '--blast <word> [<issue>...]`: it rebases onto origin/main, runs `mise run check` and ' +
      '`mise run test`, pushes the branch and opens the pull request, which is merged once CI is green.'
    )
  }
  if (adminMerge.test(code)) {
    return '`gh pr merge --admin` merges past the required checks. Wait for CI, then merge without it.'
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
