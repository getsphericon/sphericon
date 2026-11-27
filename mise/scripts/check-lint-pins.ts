// The analyzers (tools/analyzers) are compiled into golangci-lint, so the dependencies they share
// with it must be the versions golangci-lint was built with, or the custom build fails or misbehaves.
// This checks that tools/analyzers/go.mod requires the golang.org/x/tools and
// github.com/golangci/plugin-module-register versions of the golangci-lint release that mise pins.
// The release itself is read from mise by `build:lint` (`golangci-lint custom --version`).
import { execFileSync } from 'node:child_process'

const shared = ['golang.org/x/tools', 'github.com/golangci/plugin-module-register']

function output(command: string, args: string[], cwd?: string): string {
  return execFileSync(command, args, { encoding: 'utf8', cwd }).trim()
}

// The version each module a go.mod requires is required at, as `go mod edit -json` reads the file.
function requirements(goMod: string): Map<string, string> {
  const parsed: unknown = JSON.parse(output('go', ['mod', 'edit', '-json', goMod]))
  const requires: unknown[] =
    typeof parsed === 'object' &&
    parsed !== null &&
    'Require' in parsed &&
    Array.isArray(parsed.Require)
      ? parsed.Require
      : []
  return new Map(
    requires.flatMap((req): [string, string][] =>
      typeof req === 'object' &&
      req !== null &&
      'Path' in req &&
      typeof req.Path === 'string' &&
      'Version' in req &&
      typeof req.Version === 'string'
        ? [[req.Path, req.Version]]
        : [],
    ),
  )
}

const release = output('mise', ['current', 'golangci-lint'])
const download: unknown = JSON.parse(
  output('go', ['mod', 'download', '-json', `github.com/golangci/golangci-lint/v2@v${release}`]),
)
const lintGoMod =
  typeof download === 'object' &&
  download !== null &&
  'GoMod' in download &&
  typeof download.GoMod === 'string'
    ? download.GoMod
    : undefined
if (lintGoMod === undefined) {
  process.stderr.write(
    `error: go mod download did not return the go.mod of golangci-lint v${release}\n`,
  )
  process.exit(1)
}

const lintRequires = requirements(lintGoMod)
// --fix moves the analyzers to golangci-lint's versions instead of failing: `update:go` runs it
// after taking every module to its latest release.
const fix = process.argv.includes('--fix')
let failed = false
for (const path of shared) {
  const theirs = lintRequires.get(path)
  const ours = output('go', ['list', '-m', '-f', '{{.Version}}', path], 'tools/analyzers')
  if (fix && theirs !== undefined && theirs !== ours) {
    output('go', ['get', `${path}@${theirs}`], 'tools/analyzers')
    process.stdout.write(
      `tools/analyzers: ${path} ${ours} => ${theirs} (golangci-lint v${release})\n`,
    )
  } else if (theirs !== ours) {
    process.stderr.write(
      `error: tools/analyzers requires ${path} ${ours} but golangci-lint v${release} uses ${theirs ?? '?'}; ` +
        `run 'cd tools/analyzers && go get ${path}@${theirs ?? '<version>'} && go mod tidy'\n`,
    )
    failed = true
  }
}
if (fix) {
  output('go', ['mod', 'tidy'], 'tools/analyzers')
}
process.exit(failed ? 1 : 0)
