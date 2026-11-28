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
  header: ['email', 'Status'],
  rows: 2,
  malformedRows: 0,
  mapping: {
    columns: [{ kind: 'key', key: 'email' }, { kind: 'ignore' }],
    decimalSeparator: 'point',
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
  cancelRequestedAt: null,
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

test('maps a status column as refusals, and the preview counts them with the warning', async () => {
  const sent: ContactImportMappingInput[] = []
  worker.use(
    handleSiteContactImportsSaveMapping(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(UPLOADED)
    }),
    handleSiteContactImportsPreview({
      body: {
        rows: [],
        totalRows: 2,
        rowsWithoutKey: 0,
        malformedRows: 0,
        distinctTags: 0,
        newCustomFields: 0,
        optOuts: 1,
        suppressions: 0,
      },
    }),
  )
  const { screen } = await renderImport(['contacts:import'])

  await screen.getByRole('combobox', { name: 'What Status imports as' }).click()
  await screen.getByRole('option', { name: 'Refusal (unsubscribe, bounce)' }).click()
  await screen.getByRole('textbox', { name: 'Value 1 of Status' }).fill('unsubscribed')
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .poll(() => sent[0]?.columns[1])
    .toEqual({
      kind: 'refusal',
      channel: 'email',
      values: [{ value: 'unsubscribed', refusal: 'unsubscribeEverything' }],
    })
  await expect
    .element(screen.getByText('The file would record up to 1 opt-outs and 0 suppressions.'))
    .toBeVisible()
  await expect.element(screen.getByText(/cannot be undone/)).toBeVisible()
})

test('offers no suppression to a Member who cannot manage suppressions', async () => {
  const { screen } = await renderImport(['contacts:import'])

  await screen.getByRole('combobox', { name: 'What Status imports as' }).click()
  await screen.getByRole('option', { name: 'Refusal (unsubscribe, bounce)' }).click()
  await screen.getByRole('combobox', { name: 'What value 1 of Status records' }).click()

  await expect
    .element(screen.getByRole('option', { name: 'A suppression (bounced, complained)' }))
    .toBeDisabled()
})
