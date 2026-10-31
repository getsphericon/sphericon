// The contract decorators against the fixtures in testdata/: each fixture marks the statement
// that must fail with `// expect-next-line: <code>` (the statement runs to the next blank line),
// and compiling it reports exactly those diagnostics. The well-formed declarations of the
// fixtures are checked for the extension they write. Run by `mise run test:typespec`.
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { test } from 'node:test'

import { compile, getSourceLocation, NodeHost } from '@typespec/compiler'
import type { Program } from '@typespec/compiler'
import { getExtensions } from '@typespec/openapi'

const testdata = join(import.meta.dirname, 'testdata')

interface Expected {
  code: string
  first: number
  last: number
}

/** The `expect-next-line` marks of a fixture, each with the line range of its statement (1-based). */
function expectations(source: string): Expected[] {
  const lines = source.split('\n')
  const out: Expected[] = []
  for (const [i, line] of lines.entries()) {
    const mark = /\/\/ expect-next-line: (\S+)/.exec(line)
    if (mark?.[1] === undefined) {
      continue
    }
    let last = i + 1
    while (last < lines.length && lines[last]?.trim() !== '') {
      last++
    }
    out.push({ code: mark[1], first: i + 2, last })
  }
  return out
}

async function compileFixture(name: string): Promise<Program> {
  return compile(NodeHost, join(testdata, name), { noEmit: true })
}

/** Compiles the fixture and asserts that it reports exactly the diagnostics its marks expect. */
async function assertDiagnostics(name: string): Promise<Program> {
  const path = join(testdata, name)
  const program = await compileFixture(name)
  const expected = expectations(readFileSync(path, 'utf8'))
  const got = program.diagnostics.map((d) => {
    const location = getSourceLocation(d.target as Parameters<typeof getSourceLocation>[0])
    assert.ok(location, `a diagnostic with no location: ${d.code} ${d.message}`)
    const line = location.file.getLineAndCharacterOfPosition(location.pos).line + 1
    return { code: d.code, line, file: location.file.path, message: d.message }
  })
  for (const d of got) {
    assert.equal(d.file, path, `a diagnostic outside the fixture: ${d.code} ${d.message}`)
  }
  const unmatched = [...got]
  for (const e of expected) {
    const at = unmatched.findIndex(
      (d) => d.code === e.code && d.line >= e.first && d.line <= e.last,
    )
    assert.notEqual(at, -1, `${name}:${e.first}: expected ${e.code}`)
    unmatched.splice(at, 1)
  }
  assert.deepEqual(
    unmatched.map((d) => `${name}:${d.line}: ${d.code} ${d.message}`),
    [],
    'unexpected diagnostics',
  )
  return program
}

/** The value of an extension on a type of the fixture, by its TypeSpec reference. */
function extension(program: Program, reference: string, key: `x-${string}`): unknown {
  const [type] = program.resolveTypeReference(reference)
  assert.ok(type, `no type ${reference}`)
  return getExtensions(program, type).get(key)
}

await test('@mcp refuses a malformed projection and writes a well-formed one', async () => {
  const program = await assertDiagnostics('mcp.tsp')
  assert.deepEqual(extension(program, 'Fixture.tool', 'x-mcp'), {
    name: 'contacts_list',
    send: true,
  })
  assert.deepEqual(extension(program, 'Fixture.twice', 'x-mcp'), { hidden: true })
})

await test('a decorator typed by a contract enum takes only its members, once', async () => {
  await assertDiagnostics('arguments.tsp')
})

await test('on /site an operation of a Workspace declares its Permissions, and only it', async () => {
  const program = await assertDiagnostics('site.tsp')
  assert.deepEqual(extension(program, 'Fixture.members', 'x-required-permissions'), [
    'members:manage',
  ])
  assert.deepEqual(extension(program, 'Fixture.contacts', 'x-required-permissions'), [])
  assert.equal(extension(program, 'Fixture.contacts', 'x-allowed-while-withheld'), true)
  assert.equal(extension(program, 'Fixture.me', 'x-allowed-while-acceptance-pending'), true)
})

await test('on /api every operation declares its scopes, a template its argument', async () => {
  const program = await assertDiagnostics('scopes.tsp')
  assert.deepEqual(extension(program, 'Fixture.contacts', 'x-required-scopes'), ['contacts:read'])
  assert.deepEqual(extension(program, 'Fixture.me', 'x-required-scopes'), [])
  assert.deepEqual(extension(program, 'Fixture.Events.list', 'x-required-scopes'), ['events:read'])
})

await test('a raw @extension of a decorator key is refused once per declaration, any other kept', async () => {
  const program = await assertDiagnostics('raw.tsp')
  assert.equal(extension(program, 'Fixture.Contact.email', 'x-something-else'), true)
})

await test('@grantable lists every member with its @grantedWith, only in a @grantable enum', async () => {
  const program = await assertDiagnostics('grants.tsp')
  assert.deepEqual(extension(program, 'Fixture.Grantable', 'x-grant-permissions'), [
    { scope: 'things:read' },
    { scope: 'things:write', permission: 'webhooks:manage' },
  ])
  assert.equal(extension(program, 'Fixture.Every', 'x-grant-permissions'), undefined)
})
