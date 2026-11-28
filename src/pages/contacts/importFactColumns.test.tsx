import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

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
} from '../../generated/site/types.gen.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { page, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportPage } from './importPage.tsx'

// An uploaded file of a key, a consent, the time it was given, where it was given, tracking and a
// column with no header.
const UPLOADED: SiteContactImport = {
  id: testId(7),
  state: 'uploaded',
  delimiter: 'comma',
  header: ['email', 'Consent', 'When', 'Form', 'Track', ''],
  rows: 1,
  malformedRows: 0,
  mapping: {
    columns: [
      { kind: 'key', key: 'email' },
      { kind: 'ignore' },
      { kind: 'ignore' },
      { kind: 'ignore' },
      { kind: 'ignore' },
      { kind: 'ignore' },
    ],
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

// The page of the import for a Member who manages suppressions, the saves it sends in sent.
async function renderImport(sent: ContactImportMappingInput[]) {
  worker.use(
    handleSiteWorkspacesList({
      body: [workspace({ permissions: ['contacts:import', 'suppressions:manage'] })],
    }),
    handleSiteContactImportsGet({ body: UPLOADED }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
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
        newCustomFields: 0,
        optOuts: 0,
        suppressions: 0,
      },
    }),
  )
  return renderWithRouter(
    <ContactImportPage />,
    routeMount(importPageRoute, { workspaceId: 'TestWs01', importId: testId(7) }),
  )
}

// A consent column on SMS with its wording and brand read from other columns (one with no header)
// and its source given once, the
// time column of it, and an Engagement tracking column, save as the Member set them.
test('maps a consent with its evidence, its time and a tracking column', async () => {
  const sent: ContactImportMappingInput[] = []
  const { screen } = await renderImport(sent)
  const choose = async (control: string, option: string) => {
    await screen.getByRole('combobox', { name: control }).click()
    await screen.getByRole('option', { name: option, exact: true }).click()
  }

  await choose('What Consent imports as', 'Consent')
  await choose('Channel of Consent', 'SMS')
  await screen.getByRole('combobox', { name: 'Values that mean consent' }).fill('yes')
  await userEvent.keyboard('{Enter}')
  await screen.getByRole('combobox', { name: 'Or read from column' }).nth(0).click()
  await screen.getByRole('option', { name: 'Form', exact: true }).click()
  await screen.getByRole('textbox', { name: 'Source' }).fill('Signup form')
  await screen.getByRole('combobox', { name: 'Or read from column' }).nth(2).click()
  await screen.getByRole('option', { name: '#6', exact: true }).click()
  await choose('What When imports as', 'Time of a consent or refusal')
  await choose('The column When times', 'Consent')
  await choose('Date format of When', 'Date and time (RFC 3339)')
  await choose('What Track imports as', 'Engagement tracking consent')
  await screen.getByRole('combobox', { name: 'Values that mean tracking is allowed' }).fill('on')
  await userEvent.keyboard('{Enter}')
  await screen.getByRole('combobox', { name: 'Values that mean tracking is refused' }).fill('off')
  await userEvent.keyboard('{Enter}')
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .poll(() => sent[0]?.columns.slice(1))
    .toEqual([
      {
        kind: 'consent',
        channel: 'sms',
        values: ['yes'],
        evidence: {
          wording: null,
          wordingColumn: 3,
          source: 'Signup form',
          sourceColumn: null,
          brand: null,
          brandColumn: 5,
        },
      },
      { kind: 'time', of: 1, dateFormat: 'rfc3339' },
      { kind: 'ignore' },
      { kind: 'engagementTracking', channel: 'email', granted: ['on'], denied: ['off'] },
      { kind: 'ignore' },
    ])
})

// A refusal column lists its values: one is added, set to a Suppression, and the first removed.
test('adds and removes the values of a refusal column', async () => {
  const sent: ContactImportMappingInput[] = []
  const { screen } = await renderImport(sent)

  await screen.getByRole('combobox', { name: 'What Consent imports as' }).click()
  await screen.getByRole('option', { name: 'Refusal (unsubscribe, bounce)' }).click()
  await screen.getByRole('button', { name: 'Add a value' }).click()
  await screen.getByRole('textbox', { name: 'Value 2 of Consent' }).fill('bounced')
  await screen.getByRole('combobox', { name: 'What value 2 of Consent records' }).click()
  await screen.getByRole('option', { name: 'A suppression (bounced, complained)' }).click()
  await screen.getByRole('button', { name: 'Remove the value' }).first().click()
  await screen.getByRole('button', { name: 'Save and preview' }).click()

  await expect
    .poll(() => sent[0]?.columns[1])
    .toEqual({
      kind: 'refusal',
      channel: 'email',
      values: [{ value: 'bounced', refusal: 'suppression' }],
    })
})
