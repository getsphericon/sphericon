import { expect, test } from 'vitest'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { workspace } from '../test/payloads.ts'
import { workspaceOptions } from './workspace.ts'

const acme = workspace({ id: 'acme', name: 'Acme' })
const globex = workspace({ id: 'globex', name: 'Globex' })

test('the Workspace is read from the cached Workspace list, not a query of its own', () => {
  expect(workspaceOptions('acme').queryKey).toEqual(siteWorkspacesListOptions().queryKey)
})

test('the Workspace of the id is picked from the list', () => {
  expect(workspaceOptions('globex').select?.([acme, globex])).toBe(globex)
})

test('a Workspace the list does not hold is undefined', () => {
  expect(workspaceOptions('initech').select?.([acme, globex])).toBeUndefined()
})
