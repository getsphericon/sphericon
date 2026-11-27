import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import { handleSiteContactImportsUpload } from '../../generated/site/msw.gen.ts'
import type { SiteContactImport } from '../../generated/site/types.gen.ts'
import { Route as importRoute } from '../../routes/workspaces/$workspaceId/contacts/import.tsx'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactImportUploadPage } from './importUpload.tsx'

const UPLOADED: SiteContactImport = {
  id: testId(7),
  state: 'uploaded',
  delimiter: 'comma',
  header: ['email'],
  rows: 1,
  malformedRows: 0,
  mapping: {
    columns: [{ kind: 'key', key: 'email' }],
    decimalSeparator: '.',
    defaultCountry: null,
    tag: 'Imported 2026-11-27',
    onlyAddNew: false,
  },
  counts: { processed: 0, created: 0, updated: 0, refused: 0, dropped: 0 },
  reportRows: 0,
  createdAt: TIMESTAMPS.createdAt,
  startedAt: null,
  endedAt: null,
}

// The critical path of an import: the file the Member picks is sent whole as the request's
// text/csv body, and the page then opens the import to map it.
test('uploads the chosen file as text/csv and opens its import', async () => {
  const sent: { type: string | null; body: string }[] = []
  worker.use(
    handleSiteContactImportsUpload(async ({ request }) => {
      sent.push({ type: request.headers.get('Content-Type'), body: await request.text() })
      return HttpResponse.json(UPLOADED)
    }),
  )
  const { screen, navigate } = await renderWithRouter(
    <ContactImportUploadPage />,
    routeMount(importRoute, { workspaceId: 'TestWs01' }),
  )
  const file = new File(['email\nann@example.com\n'], 'contacts.csv', { type: 'text/csv' })

  // Mantine's FileInput is a button over a hidden file input, which takes the file.
  const input = screen.container.querySelector('input[type="file"]')
  expect(input).not.toBeNull()
  await userEvent.upload(input ?? document.body, file)
  await screen.getByRole('button', { name: 'Upload' }).click()

  await expect.poll(() => sent).toEqual([{ type: 'text/csv', body: 'email\nann@example.com\n' }])
  await expect
    .poll(() => navigate.mock.calls[0]?.[0])
    .toMatchObject({ params: { workspaceId: 'TestWs01', importId: testId(7) } })
})
