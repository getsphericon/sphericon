// Exits 0 when the tip of a local branch is on main, 1 when it is not.
//
//   node mise/scripts/branch-merged.ts <branch>
//
// A pull request is squashed, so its commits never reach origin/main under their own SHAs and
// `git merge-base --is-ancestor` alone would keep every merged branch. The tip also counts as merged
// when GitHub reports a merged pull request from this branch whose head is exactly that commit; a
// commit added after the merge, or an unreachable GitHub, leaves the branch alone.
import { execFileSync } from 'node:child_process'

function output(command: string, args: string[]): string {
  return execFileSync(command, args, {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
  }).trim()
}

function merged(branch: string): boolean {
  const tip = output('git', ['rev-parse', '--verify', `refs/heads/${branch}`])
  try {
    execFileSync('git', ['merge-base', '--is-ancestor', tip, 'origin/main'], { stdio: 'ignore' })
    return true
  } catch {
    // Not an ancestor: it may still have been squashed.
  }
  try {
    const heads = output('gh', [
      'pr',
      'list',
      '--head',
      branch,
      '--state',
      'merged',
      '--json',
      'headRefOid',
      '--jq',
      '.[].headRefOid',
    ])
    return heads.split('\n').includes(tip)
  } catch {
    return false
  }
}

const [branch] = process.argv.slice(2)
process.exit(branch !== undefined && merged(branch) ? 0 : 1)
