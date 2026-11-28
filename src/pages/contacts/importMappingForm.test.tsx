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
  SiteContactImport,
  SiteContactImportPreview,
} from '../../generated/site/types.gen.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { page, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportPage } from './importPage.tsx'

// An uploaded file of an email, a birthday whose format its sample left open and a status column.
const UPLOADED: SiteContactImport = {
  id: testId(7),
  state: 'uploaded',
  delimiter: 'comma',
  header: ['email', 'born', 'status'],
  rows: 2,
  malformedRows: 0,
  mapping: {
    columns: [
      { kind: 'key', key: 'email' },
      { kind: 'attribute', attribute: 'birthday', dateFormat: null },
      { kind: 'ignore' },
    ],
    decimalSeparator: 'point',
    defaultCountry: null,
    tag: 'Imported 2026-11-27',
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

const PREVIEW: SiteContactImportPreview = {
  rows: [
    {
      row: 1,
      cells: ['ann@example.com', '03/04/1990', ''],
      refusal: null,
      dropped: [],
      warnings: [],
    },
  ],
  totalRows: 2,
  rowsWithoutKey: 0,
  malformedRows: 0,
  distinctTags: 1,
  newCustomFields: 0,
  optOuts: 0,
  suppressions: 0,
}

// The page of the uploaded import for an Owner, the saves it sends collected in sent.
async function renderMapping(sent: ContactImportMappingInput[], refuse?: Record<string, string[]>) {
  worker.use(
    handleSiteWorkspacesList({
      body: [
        workspace({
          permissions: ['contacts:import', 'custom_fields:manage', 'suppressions:manage'],
        }),
      ],
    }),
    handleSiteContactImportsGet({ body: UPLOADED }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
    handleSiteContactImportsSaveMapping(async ({ request }) => {
      sent.push(await request.json())
      return refuse === undefined
        ? HttpResponse.json(UPLOADED)
        : problem(422, { title: 'Unprocessable', errors: refuse })
    }),
    handleSiteContactImportsPreview({ body: PREVIEW }),
  )
  return renderWithRouter(
    <ContactImportPage />,
    routeMount(importPageRoute, { workspaceId: 'TestWs01', importId: testId(7) }),
  )
}

// A date column without its format is pointed out under its format on save, and nothing is sent
// until the Member chooses one; then the mapping saves, its preview shows and the import may run.
test('a date column without a format is pointed out under it before anything is saved', async () => {
  const sent: ContactImportMappingInput[] = []
  const { screen } = await renderMapping(sent)

  await expect.element(screen.getByRole('button', { name: 'Run import' })).toBeDisabled()
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect.element(screen.getByText('Choose the date format.')).toBeVisible()
  expect(sent).toEqual([])
  await screen.getByRole('combobox', { name: 'Date format of born' }).click()
  await screen.getByRole('option', { name: 'dd/mm/yyyy' }).click()
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .poll(() => sent[0]?.columns[1])
    .toEqual({ kind: 'attribute', attribute: 'birthday', dateFormat: 'dd/mm/yyyy' })
  await expect.element(screen.getByText('ann@example.com')).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Run import' })).toBeEnabled()
})

// A save the server refuses for one column shows its message under that column, not in an alert.
test('a save refused for a column shows the refusal under the column', async () => {
  const sent: ContactImportMappingInput[] = []
  const { screen } = await renderMapping(sent, {
    'columns.2': ['Two columns write the same value.'],
  })
  await screen.getByRole('combobox', { name: 'Date format of born' }).click()
  await screen.getByRole('option', { name: 'dd/mm/yyyy' }).click()

  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect.element(screen.getByText('Two columns write the same value.')).toBeVisible()
  await expect.element(screen.getByText('Could not save the mapping')).not.toBeInTheDocument()
})

// A refusal column on Telegram turned into a consent column is never saved as one of email: the
// form asks for the Channel under the column and sends nothing.
test('a consent column left on Telegram asks for its channel and saves nothing', async () => {
  const sent: ContactImportMappingInput[] = []
  const { screen } = await renderMapping(sent)
  await screen.getByRole('combobox', { name: 'Date format of born' }).click()
  await screen.getByRole('option', { name: 'dd/mm/yyyy' }).click()
  await screen.getByRole('combobox', { name: 'What status imports as' }).click()
  await screen.getByRole('option', { name: 'Refusal (unsubscribe, bounce)' }).click()
  await screen.getByRole('combobox', { name: 'Channel of status' }).click()
  await screen.getByRole('option', { name: 'Telegram' }).click()
  await screen.getByRole('combobox', { name: 'What status imports as' }).click()
  await screen.getByRole('option', { name: 'Consent', exact: true }).click()

  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .element(screen.getByText('Consent and tracking are on email or SMS: choose the channel.'))
    .toBeVisible()
  expect(sent).toEqual([])
})
