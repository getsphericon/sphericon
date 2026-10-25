// Lands the work of this worktree on main, the way AGENTS.md describes it: rebase onto a freshly
// fetched origin/main, the checks and the tests, `git push origin HEAD:main`, then the ticket check.
//
//   mise run land -- [--skip-test] [<issue>...]
//
// A worktree lands from a detached HEAD, never a branch, with a clean tree. --skip-test is for a
// change no Go or frontend test reads (a doc, a workflow). Each <issue> is the ticket the work was
// for: once the push is done it is closed with a comment unless GitHub closed it from the commit's
// `Closes #<n>` footer. The worktree itself is removed by the caller, which is standing in it.
import { execFileSync } from 'node:child_process'

function run(command: string, args: string[]): void {
  execFileSync(command, args, { stdio: 'inherit' })
}

function output(command: string, args: string[]): string {
  return execFileSync(command, args, { encoding: 'utf8' }).trim()
}

function fail(message: string): never {
  process.stderr.write(`land: ${message}\n`)
  process.exit(1)
}

const args = process.argv.slice(2)
const skipTest = args.includes('--skip-test')
const issues = args.filter((arg) => arg !== '--skip-test')

const branch = output('git', ['branch', '--show-current'])
if (branch !== '') {
  fail(`HEAD is on the branch '${branch}'; work lands from a detached HEAD, never a branch`)
}
if (output('git', ['status', '--porcelain']) !== '') {
  fail('the working tree is not clean; commit or discard the changes first')
}

run('git', ['fetch', '-q', 'origin'])
run('git', ['rebase', 'origin/main'])
if (output('git', ['rev-parse', 'HEAD']) === output('git', ['rev-parse', 'origin/main'])) {
  fail('nothing to land, HEAD is origin/main')
}

run('mise', ['run', 'check'])
if (!skipTest) {
  run('mise', ['run', 'test'])
}

run('git', ['push', 'origin', 'HEAD:main'])
const sha = output('git', ['rev-parse', '--short', 'HEAD'])

for (const issue of issues) {
  const state = output('gh', ['issue', 'view', issue, '--json', 'state', '--jq', '.state'])
  if (state !== 'CLOSED') {
    run('gh', ['issue', 'close', issue, '--comment', `Implemented on main in ${sha}.`])
  }
  const after = output('gh', ['issue', 'view', issue, '--json', 'state', '--jq', '.state'])
  process.stdout.write(`issue #${issue}: ${after}\n`)
}

process.stdout.write(
  `landed ${sha}. Remove this worktree from the primary checkout (it drops the databases too), then update it:\n` +
    `  mise run worktrees:rm ${output('git', ['rev-parse', '--show-toplevel'])}\n` +
    '  git fetch origin && git rebase origin/main\n',
)
