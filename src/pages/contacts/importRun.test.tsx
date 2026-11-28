import { expect, test } from 'vitest'

import {
  handleSiteContactImportsGet,
  handleSiteContactImportsReportLink,
} from '../../generated/site/msw.gen.ts'
import type { SiteContactImport } from '../../generated/site/types.gen.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportPage } from './importPage.tsx'

// An import that ended: it wrote what its counts say, recorded refusals and reported a row.
const ENDED: SiteContactImport = {
  id: testId(7),
  state: 'done',
  delimiter: 'comma',
  header: ['email', 'status'],
  rows: 3,
  malformedRows: 1,
  mapping: {
    columns: [{ kind: 'key', key: 'email' }, { kind: 'ignore' }],
    decimalSeparator: 'point',
    defaultCountry: null,
    tag: null,
    onlyAddNew: false,
    asNewSignUps: false,
  },
  counts: {
    processed: 3,
    created: 2,
    updated: 0,
    refused: 1,
    dropped: 0,
    optedOut: 1,
    suppressed: 1,
  },
  reportRows: 1,
  createdAt: TIMESTAMPS.createdAt,
  startedAt: TIMESTAMPS.createdAt,
  endedAt: TIMESTAMPS.updatedAt,
  cancelRequestedAt: null,
}

async function renderImport(imp: SiteContactImport) {
  worker.use(handleSiteContactImportsGet({ body: imp }))
  return renderWithRouter(
    <ContactImportPage />,
    routeMount(importPageRoute, { workspaceId: 'TestWs01', importId: testId(7) }),
  )
}

// An ended import says what it wrote and recorded; a report whose link is refused says so.
test('an ended import shows its counts, and a report link refused says why', async () => {
  worker.use(
    handleSiteContactImportsReportLink(() => problem(404, { detail: 'The report is gone.' })),
  )
  const { screen } = await renderImport(ENDED)

  await expect
    .element(screen.getByText('2 created, 0 updated, 1 refused, 0 values dropped.'))
    .toBeVisible()
  await expect.element(screen.getByText('1 opt-outs and 1 suppressions recorded.')).toBeVisible()
  await expect.element(screen.getByText(/1 row was refused/)).toBeVisible()
  await screen.getByRole('button', { name: 'Download refused rows' }).click()

  await expect.element(screen.getByText('Could not download the report')).toBeVisible()
  await expect.element(screen.getByText('The report is gone.')).toBeVisible()
})

// A cancelled import says so and that what it wrote stays; with nothing reported, no download.
test('a cancelled import says what stays, and offers no report it does not have', async () => {
  const { screen } = await renderImport({
    ...ENDED,
    state: 'cancelled',
    malformedRows: 0,
    reportRows: 0,
    counts: { ...ENDED.counts, optedOut: 0, suppressed: 0 },
  })

  await expect
    .element(screen.getByText('The import was cancelled. The contacts it wrote stay.'))
    .toBeVisible()
  await expect
    .element(screen.getByRole('button', { name: 'Download refused rows' }))
    .not.toBeInTheDocument()
  await expect.element(screen.getByText(/opt-outs and/)).not.toBeInTheDocument()
})

// A running import with no rows shows an empty progress, never a division by zero.
test('a running import of no rows shows its progress', async () => {
  const { screen } = await renderImport({
    ...ENDED,
    state: 'running',
    rows: 0,
    endedAt: null,
    counts: { ...ENDED.counts, processed: 0 },
  })

  await expect.element(screen.getByText('0 of 0 rows read')).toBeVisible()
})
