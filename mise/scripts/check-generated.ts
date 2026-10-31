// .gitattributes is the one list of generated files: its `linguist-generated` patterns. This checks
// that the list and the files agree, in both directions:
// - every tracked file with a generated header (Go's `// Code generated … DO NOT EDIT.` line, or
//   `// @ts-nocheck` on the frontend, in its first lines) is marked `linguist-generated`;
// - every `linguist-generated` pattern still matches a tracked file.
// A file under a `testdata/` directory is a fixture of a test, whatever its header says.
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

const header = String.raw`^(// Code generated .* DO NOT EDIT\.|// @ts-nocheck)$`
const headLines = 5

function git(args: string[], input?: string): string {
  return execFileSync('git', args, { encoding: 'utf8', input, maxBuffer: 64 * 1024 * 1024 })
}

// `git grep -n -z` prints <path> NUL <line> NUL <text> per match; it exits 1 on no match.
function grep(pattern: string): string {
  try {
    return git(['grep', '-n', '-z', '-E', '-e', pattern])
  } catch (error) {
    if (typeof error === 'object' && error !== null && 'status' in error && error.status === 1) {
      return ''
    }
    throw error
  }
}

const withHeader = [
  ...new Set(
    grep(header)
      .split('\n')
      .map((match) => match.split('\0'))
      .filter(
        ([path, line]) =>
          path !== undefined &&
          path !== '' &&
          Number(line) <= headLines &&
          !/(?:^|\/)testdata\//.test(path),
      )
      .map(([path]) => path),
  ),
]

const errors: string[] = []

// `git check-attr -z` prints <path> NUL <attribute> NUL <value> NUL per path.
const attrs = git(['check-attr', '-z', '--stdin', 'linguist-generated'], withHeader.join('\0'))
  .split('\0')
  .slice(0, -1)
for (let i = 0; i < attrs.length; i += 3) {
  if (attrs[i + 2] !== 'true') {
    errors.push(
      `${attrs[i]} has a generated header, but no .gitattributes pattern marks it linguist-generated`,
    )
  }
}

for (const line of readFileSync('.gitattributes', 'utf8').split('\n')) {
  const [pattern, ...attributes] = line.trim().split(/\s+/)
  if (
    pattern === undefined ||
    pattern === '' ||
    pattern.startsWith('#') ||
    !attributes.some((attribute) => attribute.startsWith('linguist-generated'))
  ) {
    continue
  }
  // A gitattributes pattern without a slash matches a name at any depth.
  const glob = pattern.includes('/') ? pattern.replace(/^\//, '') : `**/${pattern}`
  if (git(['ls-files', '--', `:(glob)${glob}`]) === '') {
    errors.push(`.gitattributes: ${pattern} matches no tracked file; remove its line`)
  }
}

for (const error of errors) {
  process.stderr.write(`error: ${error}\n`)
}
process.exit(errors.length > 0 ? 1 : 0)
