import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteContactImportsCancel,
  handleSiteContactImportsGet,
} from '../../generated/site/msw.gen.ts'
import type { SiteContactImport } from '../../generated/site/types.gen.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportPage } from './importPage.tsx'

const RUNNING: SiteContactImport = {
  id: testId(7),
  state: 'running',
  delimiter: 'comma',
  header: ['email'],
  rows: 2500,
  malformedRows: 0,
  mapping: {
    columns: [{ kind: 'key', key: 'email' }],
    decimalSeparator: '.',
    defaultCountry: null,
    tag: null,
    onlyAddNew: false,
    asNewSignUps: false,
  },
  counts: {
    processed: 1000,
    created: 999,
    updated: 0,
    refused: 1,
    dropped: 0,
    optedOut: 0,
    suppressed: 0,
  },
  reportRows: 1,
  createdAt: TIMESTAMPS.createdAt,
  startedAt: TIMESTAMPS.createdAt,
  endedAt: null,
  cancelRequestedAt: null,
}

async function renderImport() {
  return renderWithRouter(
    <ContactImportPage />,
    routeMount(importPageRoute, { workspaceId: 'TestWs01', importId: testId(7) }),
  )
}

// A running import shows how far its job has read and stops on a confirmed Cancel, the page then
// saying it stops after the rows it is writing.
test('cancels a running import after a confirmation', async () => {
  let imp = RUNNING
  worker.use(
    handleSiteContactImportsGet(() => HttpResponse.json(imp)),
    handleSiteContactImportsCancel(() => {
      imp = { ...RUNNING, cancelRequestedAt: TIMESTAMPS.updatedAt }
      return HttpResponse.json(imp)
    }),
  )
  const { screen } = await renderImport()

  await expect.element(screen.getByText('1000 of 2500 rows read')).toBeVisible()
  await screen.getByRole('button', { name: 'Cancel import' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Cancel import' }).click()

  await expect.element(screen.getByText('Stopping after the rows it is writing…')).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Cancel import' })).toBeDisabled()
})

// An import whose job's tries were spent says it failed, keeps what it wrote and offers its report.
test('shows a failed import with its report', async () => {
  worker.use(
    handleSiteContactImportsGet(() =>
      HttpResponse.json({ ...RUNNING, state: 'failed', endedAt: TIMESTAMPS.updatedAt }),
    ),
  )
  const { screen } = await renderImport()

  await expect.element(screen.getByText(/The import failed after several tries/)).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Download refused rows' })).toBeVisible()
})
