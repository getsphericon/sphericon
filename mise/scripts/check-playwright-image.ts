// The frontend tests run in CI inside the Playwright image (.github/workflows/ci.yml, test-frontend),
// which ships the browser and the system libraries of one Playwright release. This checks that the
// image is the release package.json pins, so an upgrade of the package fails here
// until the image follows it, instead of testing with a browser the package did not ship with.
import { readFileSync } from 'node:fs'

const workflow = '.github/workflows/ci.yml'

const manifest: unknown = JSON.parse(readFileSync('package.json', 'utf8'))
const pinned =
  typeof manifest === 'object' &&
  manifest !== null &&
  'devDependencies' in manifest &&
  typeof manifest.devDependencies === 'object' &&
  manifest.devDependencies !== null &&
  'playwright' in manifest.devDependencies &&
  typeof manifest.devDependencies.playwright === 'string'
    ? manifest.devDependencies.playwright
    : undefined
if (pinned === undefined || !/^\d+\.\d+\.\d+$/.test(pinned)) {
  process.stderr.write(`error: package.json pins playwright to an exact version, not ${pinned}\n`)
  process.exit(1)
}

const images = [
  ...readFileSync(workflow, 'utf8').matchAll(
    /mcr\.microsoft\.com\/playwright:v([^@\s-]+)[^@\s]*@sha256:[0-9a-f]{64}/g,
  ),
].map((m) => m[1])
if (images.length === 0) {
  process.stderr.write(`error: ${workflow} names no Playwright image pinned by a digest\n`)
  process.exit(1)
}
const stale = images.filter((version) => version !== pinned)
if (stale.length > 0) {
  process.stderr.write(
    `error: ${workflow} runs the Playwright image ${stale.join(', ')}, package.json pins ${pinned}: ` +
      `set the image to v${pinned}-noble and its digest ` +
      `(the Docker-Content-Digest of https://mcr.microsoft.com/v2/playwright/manifests/v${pinned}-noble)\n`,
  )
  process.exit(1)
}
