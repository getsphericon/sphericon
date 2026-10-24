// The analyzers (tools/analyzers) are compiled into golangci-lint, so the dependencies they share
// with it must be the versions golangci-lint was built with, or the custom build fails or misbehaves.
// This checks that tools/analyzers/go.mod requires the golang.org/x/tools and
// github.com/golangci/plugin-module-register versions of the golangci-lint release that mise pins.
// The release itself is read from mise by `build:lint` (`golangci-lint custom --version`).
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

const shared = ['golang.org/x/tools', 'github.com/golangci/plugin-module-register']

function output(command: string, args: string[], cwd?: string): string {
  return execFileSync(command, args, { encoding: 'utf8', cwd }).trim()
}

// The version a go.mod requires a Go module at: its require line, in a block or on its own.
function required(goMod: string, path: string): string | undefined {
  const escaped = path.replaceAll(/[.*+?^${}()|[\]\\/]/g, String.raw`\$&`)
  return new RegExp(String.raw`^\s*(?:require\s+)?${escaped}\s+(\S+)`, 'm').exec(goMod)?.[1]
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

const lintText = readFileSync(lintGoMod, 'utf8')
let failed = false
for (const path of shared) {
  const theirs = required(lintText, path)
  const ours = output('go', ['list', '-m', '-f', '{{.Version}}', path], 'tools/analyzers')
  if (theirs !== ours) {
    process.stderr.write(
      `error: tools/analyzers requires ${path} ${ours} but golangci-lint v${release} uses ${theirs ?? '?'}; ` +
        `run 'cd tools/analyzers && go get ${path}@${theirs ?? '<version>'} && go mod tidy'\n`,
    )
    failed = true
  }
}
process.exit(failed ? 1 : 0)
