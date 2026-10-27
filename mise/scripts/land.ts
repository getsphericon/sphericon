// Opens the pull request for the work of this worktree, the way AGENTS.md describes it: rebase onto a
// freshly fetched origin/main, the checks and the tests, the pull request body and title checks, then
// push the branch and open (or reuse) the pull request. Nothing reaches main from here: it is merged
// on GitHub once CI is green.
//
//   mise run land -- --door <one-way|two-way> --blast <word> [--ai <none|helped|wrote>]
//     [--title <conventional commit>] [--skip-test] [<issue>...]
//
// A worktree lands from its own branch with a clean tree. --skip-test is for a change no Go or
// frontend test reads (a doc, a workflow). Each <issue> is the ticket the work was for: the body says
// `Closes #<n>` and GitHub closes it when the pull request is merged. The title defaults to the
// subject of the branch's only commit; with several commits pass --title, because the repository
// squashes with it and release-please reads it. --door and --blast fill the template's "Merge Danger".
import { execFileSync } from 'node:child_process'

type Ai = 'none' | 'helped' | 'wrote'

const aiLines: Record<Ai, string> = {
  none: 'No AI tool was used.',
  helped: 'An AI tool helped; I reviewed the change and understand every line.',
  wrote: 'An AI tool wrote most of the change; I reviewed it and understand every line.',
}

function run(command: string, args: string[], env: Record<string, string> = {}): void {
  execFileSync(command, args, { stdio: 'inherit', env: { ...process.env, ...env } })
}

function output(command: string, args: string[]): string {
  return execFileSync(command, args, { encoding: 'utf8' }).trim()
}

function fail(message: string): never {
  process.stderr.write(`land: ${message}\n`)
  process.exit(1)
}

function takeValue(args: string[], flag: string): string | undefined {
  const at = args.indexOf(flag)
  if (at === -1) {
    return undefined
  }
  const [, value] = args.splice(at, 2)
  if (value === undefined || value.startsWith('--')) {
    fail(`${flag} needs a value`)
  }
  return value
}

function takeFlag(args: string[], flag: string): boolean {
  const at = args.indexOf(flag)
  if (at === -1) {
    return false
  }
  args.splice(at, 1)
  return true
}

function isAi(value: string): value is Ai {
  return value in aiLines
}

const args = process.argv.slice(2)
const skipTest = takeFlag(args, '--skip-test')
const door = takeValue(args, '--door')
const blast = takeValue(args, '--blast')
const aiValue = takeValue(args, '--ai') ?? 'wrote'
const titleFlag = takeValue(args, '--title')
const issues = args

if (!isAi(aiValue)) {
  fail(`--ai is one of none, helped, wrote, not '${aiValue}'`)
}
if (door !== 'one-way' && door !== 'two-way') {
  fail("--door is 'one-way' or 'two-way'")
}
if (blast === undefined) {
  fail('--blast is a one-word description of the blast radius')
}
for (const issue of issues) {
  if (!/^\d+$/.test(issue)) {
    fail(`'${issue}' is not an issue number`)
  }
}

const branch = output('git', ['branch', '--show-current'])
if (branch === '' || branch === 'main') {
  fail(`HEAD is on '${branch || 'a detached HEAD'}'; work lands from its own branch, never main`)
}
if (output('git', ['status', '--porcelain']) !== '') {
  fail('the working tree is not clean; commit or discard the changes first')
}

run('git', ['fetch', '-q', 'origin'])
run('git', ['rebase', 'origin/main'])
if (output('git', ['rev-parse', 'HEAD']) === output('git', ['rev-parse', 'origin/main'])) {
  fail('nothing to land, HEAD is origin/main')
}

const subjects = output('git', ['log', '--reverse', '--format=%s', 'origin/main..HEAD']).split('\n')
const [onlySubject] = subjects
const title = titleFlag ?? (subjects.length === 1 ? onlySubject : undefined)
if (title === undefined) {
  fail(`${subjects.length} commits: pass --title, the squashed commit takes it`)
}

const evidence = skipTest
  ? '`mise run check` passed'
  : '`mise run check` and `mise run test` passed'
const body = [
  '## Issue',
  '',
  issues.length === 0 ? 'None.' : issues.map((issue) => `Closes #${issue}`).join('\n'),
  '',
  '## Summary',
  '',
  subjects.map((subject) => `- ${subject}`).join('\n'),
  '',
  '## Evidence',
  '',
  `- **After:** ${evidence} on the rebased branch.`,
  '',
  '## Merge Danger',
  '',
  `**Door:** ${door}`,
  '',
  `**Blast Radius:** ${blast}`,
  '',
  '## AI assistance',
  '',
  ...Object.entries(aiLines).map(([key, line]) => `- [${key === aiValue ? 'x' : ' '}] ${line}`),
  '',
].join('\n')

run('mise', ['run', 'check'])
if (!skipTest) {
  run('mise', ['run', 'test'])
}
// What CI runs on the title and the body, before the branch leaves the machine.
run('mise', ['run', 'check:pr-title'], { PR_TITLE: title })
run('mise', ['run', 'check:pr-body'], { PR_BODY: body, BASE: 'origin/main' })

run('git', ['push', '-u', 'origin', 'HEAD', '--force-with-lease'])

const existing = output('gh', [
  'pr',
  'list',
  '--head',
  branch,
  '--state',
  'open',
  '--json',
  'url',
  '--jq',
  '.[0].url // ""',
])
if (existing === '') {
  run('gh', ['pr', 'create', '--base', 'main', '--head', branch, '--title', title, '--body', body])
} else {
  run('gh', ['pr', 'edit', existing, '--title', title, '--body', body])
  process.stdout.write(`updated ${existing}\n`)
}

process.stdout.write(
  `Pushed ${branch}. Merge the pull request once CI is green (squash, the title is the commit). ` +
    `Afterwards, from the primary checkout (it drops the databases too):\n` +
    `  mise run worktrees:rm ${output('git', ['rev-parse', '--show-toplevel'])}\n` +
    '  git fetch origin && git rebase origin/main\n',
)
