// Keeps one open issue that lists the mise tools whose newest release is not the one mise.lock
// pins (Dependabot has no mise ecosystem), and closes it once the lock is current. Run weekly by
// .github/workflows/tools.yml with GH_TOKEN set; needs mise and gh on the PATH.
import { execFileSync } from 'node:child_process'

const title = 'Update the mise tools'
const label = 'ready-for-agent'

interface Outdated {
  name: string
  current: string
  latest: string
  release_url?: string | null
}

function output(command: string, args: string[]): string {
  return execFileSync(command, args, { encoding: 'utf8' }).trim()
}

function isOutdated(value: unknown): value is Outdated {
  return (
    typeof value === 'object' &&
    value !== null &&
    'name' in value &&
    typeof value.name === 'string' &&
    'current' in value &&
    typeof value.current === 'string' &&
    'latest' in value &&
    typeof value.latest === 'string'
  )
}

function outdatedTools(): Outdated[] {
  const parsed: unknown = JSON.parse(output('mise', ['outdated', '--local', '--json']))
  if (typeof parsed !== 'object' || parsed === null) {
    throw new Error('mise outdated --json did not return an object')
  }
  return Object.entries(parsed)
    .toSorted(([a], [b]) => a.localeCompare(b))
    .map(([, tool]): unknown => tool)
    .filter((tool): tool is Outdated => isOutdated(tool))
}

// The number of the open issue with this title, if any: the search matches on words, not exactly.
function openIssue(): string | undefined {
  const issues: unknown = JSON.parse(
    output('gh', [
      'issue',
      'list',
      '--state',
      'open',
      '--label',
      label,
      '--search',
      `"${title}" in:title`,
      '--json',
      'number,title',
    ]),
  )
  if (!Array.isArray(issues)) {
    return undefined
  }
  const found = issues.find(
    (issue: unknown) =>
      typeof issue === 'object' && issue !== null && 'title' in issue && issue.title === title,
  )
  return typeof found === 'object' && found !== null && 'number' in found
    ? String(found.number)
    : undefined
}

function bodyOf(tools: Outdated[]): string {
  const rows = tools.map((tool) => {
    const newest = tool.release_url ? `[${tool.latest}](${tool.release_url})` : tool.latest
    return `| ${tool.name} | ${tool.current} | ${newest} |`
  })
  return `These tools are declared as \`latest\` in \`.mise.toml\`, so \`mise.lock\` is the only thing that
holds them back, and nothing moves it but a commit.

| Tool | Locked | Newest |
|---|---|---|
${rows.join('\n')}

Bump each with \`mise upgrade <tool>\` (it rewrites \`mise.lock\`), read the release notes of a tool
that fails \`mise run check\`, and land it with a \`build(deps):\` commit. The newest release may be
withheld by mise's release-age cutoff; if \`mise upgrade\` skips a tool, leave it for the next run.
`
}

const tools = outdatedTools()
const number = openIssue()

if (tools.length === 0) {
  if (number !== undefined) {
    execFileSync(
      'gh',
      ['issue', 'close', number, '--comment', 'mise.lock pins the newest release of every tool.'],
      { stdio: 'inherit' },
    )
  }
} else if (number === undefined) {
  execFileSync(
    'gh',
    ['issue', 'create', '--title', title, '--label', label, '--body', bodyOf(tools)],
    { stdio: 'inherit' },
  )
} else {
  execFileSync('gh', ['issue', 'edit', number, '--body', bodyOf(tools)], { stdio: 'inherit' })
}
