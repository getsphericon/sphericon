import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteContactImportsGet,
  handleSiteContactImportsPreview,
  handleSiteContactImportsSaveMapping,
  handleSiteCustomFieldsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  ContactImportMappingInput,
  Permission,
  SiteContactImport,
} from '../../generated/site/types.gen.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { page, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportPage } from './importPage.tsx'

const UPLOADED: SiteContactImport = {
  id: testId(7),
  state: 'uploaded',
  delimiter: 'comma',
  header: ['email', 'Joined'],
  rows: 1,
  malformedRows: 0,
  mapping: {
    columns: [{ kind: 'key', key: 'email' }, { kind: 'ignore' }],
    decimalSeparator: '.',
    defaultCountry: null,
    tag: null,
    onlyAddNew: false,
    asNewSignUps: false,
  },
  counts: {
    processed: 0,
    created: 0,
    updated: 0,
    refused: 0,
    dropped: 0,
    optedOut: 0,
    suppressed: 0,
  },
  reportRows: 0,
  createdAt: TIMESTAMPS.createdAt,
  startedAt: null,
  endedAt: null,
}

// The import's page for a Member holding permissions, its Workspace with no Custom field.
async function renderImport(permissions: Permission[]) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace({ permissions })] }),
    handleSiteContactImportsGet({ body: UPLOADED }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
  )
  return renderWithRouter(
    <ContactImportPage />,
    routeMount(importPageRoute, { workspaceId: 'TestWs01', importId: testId(7) }),
  )
}

test('maps a column to a new datetime field, and its preview counts the new field', async () => {
  const sent: ContactImportMappingInput[] = []
  worker.use(
    handleSiteContactImportsSaveMapping(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(UPLOADED)
    }),
    handleSiteContactImportsPreview({
      body: {
        rows: [],
        totalRows: 1,
        rowsWithoutKey: 0,
        malformedRows: 0,
        distinctTags: 0,
        newCustomFields: 1,
        optOuts: 0,
        suppressions: 0,
      },
    }),
  )
  const { screen } = await renderImport(['contacts:import', 'custom_fields:manage'])

  await screen.getByRole('combobox', { name: 'What Joined imports as' }).click()
  await screen.getByRole('option', { name: 'New custom field' }).click()
  await screen.getByRole('combobox', { name: 'Type of the new field for Joined' }).click()
  await screen.getByRole('option', { name: 'Date and time' }).click()
  await screen.getByRole('combobox', { name: 'Date format of Joined' }).click()
  await screen.getByRole('option', { name: 'dd/mm/yyyy' }).click()
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .poll(() => sent[0]?.columns[1])
    .toEqual({
      kind: 'newCustomField',
      key: 'joined',
      label: 'Joined',
      type: 'datetime',
      dateFormat: 'dd/mm/yyyy',
    })
  await expect.element(screen.getByText(/1 new custom fields/)).toBeVisible()
})

test('offers no new field to a Member who cannot manage Custom fields', async () => {
  const { screen } = await renderImport(['contacts:import'])

  await screen.getByRole('combobox', { name: 'What Joined imports as' }).click()

  await expect.element(screen.getByRole('option', { name: 'New custom field' })).toBeDisabled()
  await expect
    .element(screen.getByText('Creating a custom field needs a role that manages custom fields.'))
    .toBeVisible()
})
