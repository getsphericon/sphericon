// Builds dist/emaildoc.js: the editor's own export code as one IIFE for the Go binary's embedded
// JavaScript runtime (goja), and dist/schema.json: the node and mark listing derived from the same
// extensions. `mise run build:emaildoc` runs it and copies both into internal/emaildoc/.
import { mkdirSync, readdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { runInThisContext } from 'node:vm'

import { build } from 'esbuild'

const editorDist = realpathSync(join(import.meta.dirname, 'node_modules/@react-email/editor/dist'))

// The image node is not a public export of @react-email/editor: it sits in an internal chunk whose
// file name carries a content hash, so the chunk is found by the symbol it exports.
function imageChunk() {
  const found = readdirSync(editorDist).filter(
    (name) =>
      name.startsWith('extension-') &&
      name.endsWith('.mjs') &&
      readFileSync(join(editorDist, name), 'utf8').includes('export { createImageExtension as t }'),
  )
  if (found.length !== 1) {
    throw new Error(
      `expected one editor chunk exporting createImageExtension, found ${found.length}`,
    )
  }
  return join(editorDist, found[0])
}

// Source rewrites for what goja cannot run. Each must match exactly once, so a dependency update that
// moves the code fails the build instead of silently shipping the unpatched version.
//
// - goja has no dynamic import(). The only one in the render path is the code block's lazy Prism
//   language loader, which a server render never reaches: it becomes a rejected promise.
// - goja bug: `split` with a lookahead regex returns extra empty strings, which breaks
//   prosemirror-model's content-expression tokenizer (`"(tableCell | tableHeader)+"`). The
//   equivalent match over word and punctuation tokens does not.
const rewrites = [
  {
    file: /@react-email[\\/]editor[\\/]dist[\\/].*\.mjs$/,
    pattern: /import\(\s*(?:\/\*[\s\S]*?\*\/\s*)?`[^`]*`\s*\)/g,
    replacement: "Promise.reject(new Error('dynamic import is not available'))",
  },
  {
    file: /prosemirror-model[\\/]dist[\\/]index\.js$/,
    pattern: /string\.split\(\/\\s\*\(\?=\\b\|\\W\|\$\)\/\)/g,
    replacement: '(string.match(/\\w+|[^\\s\\w]/g) || [])',
  },
]

const rewritten = new Map()
const rewriteSources = {
  name: 'rewrite-sources',
  setup(b) {
    for (const rewrite of rewrites) {
      b.onLoad({ filter: rewrite.file }, (args) => {
        const contents = readFileSync(args.path, 'utf8').replace(rewrite.pattern, () => {
          rewritten.set(rewrite, (rewritten.get(rewrite) ?? 0) + 1)
          return rewrite.replacement
        })
        return { contents, loader: 'js' }
      })
    }
  },
}

const resolveImageChunk = {
  name: 'resolve-image-chunk',
  setup(b) {
    b.onResolve({ filter: /^editor-image-extension$/ }, () => ({ path: imageChunk() }))
  },
}

const outdir = join(import.meta.dirname, 'dist')
mkdirSync(outdir, { recursive: true })

await build({
  entryPoints: [join(import.meta.dirname, 'src/entry.mjs')],
  outfile: join(outdir, 'emaildoc.js'),
  bundle: true,
  format: 'iife',
  platform: 'browser',
  target: 'es2020',
  minify: true,
  legalComments: 'none',
  define: { 'process.env.NODE_ENV': '"production"' },
  banner: { js: readFileSync(join(import.meta.dirname, 'src/shims.js'), 'utf8') },
  plugins: [rewriteSources, resolveImageChunk],
  logLevel: 'warning',
})

for (const rewrite of rewrites) {
  if (rewritten.get(rewrite) !== 1) {
    throw new Error(
      `expected one match of ${rewrite.pattern} in ${rewrite.file}, got ${rewritten.get(rewrite) ?? 0}`,
    )
  }
}

// The listing for agents comes out of the bundle itself, so it can never describe other extensions
// than the ones that render.
runInThisContext(readFileSync(join(outdir, 'emaildoc.js'), 'utf8'))
writeFileSync(
  join(outdir, 'schema.json'),
  `${JSON.stringify(globalThis.emaildoc.schema(), null, 2)}\n`,
)
