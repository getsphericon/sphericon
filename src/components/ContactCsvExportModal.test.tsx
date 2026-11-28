import { expect, test, vi } from 'vitest'

import { handleSiteContactCsvExportsCreate } from '../generated/site/msw.gen.ts'
import type { SiteContactCsvExportInput } from '../generated/site/types.gen.ts'
import { testId } from '../test/payloads.ts'
import { problem } from '../test/problem.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { worker } from '../test/worker.ts'
import { ContactCsvExportModal } from './ContactCsvExportModal.tsx'

// Without a Segment the export is of every Contact; Cancel closes it.
test('describes the export of every contact, and Cancel closes it', async () => {
  const closed = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <ContactCsvExportModal workspaceId="TestWs01" opened onClose={closed} />,
  )

  await expect
    .element(screen.getByText('Every contact of the workspace, one row each', { exact: false }))
    .toBeVisible()
  await screen.getByRole('button', { name: 'Cancel' }).click()

  await expect.poll(() => closed.mock.calls.length).toBe(1)
})

// The export of a Segment's Contacts names the Segment and the columns chosen; one the Member may
// not make says which Permission it needs.
test("exports a Segment's Contacts with the columns chosen, and says when it is refused", async () => {
  const sent: SiteContactCsvExportInput[] = []
  const closed = vi.fn<() => void>()
  worker.use(
    handleSiteContactCsvExportsCreate(async ({ request }) => {
      sent.push(await request.json())
      return problem(403, { detail: 'contacts:export required' })
    }),
  )
  const screen = await renderWithProviders(
    <ContactCsvExportModal workspaceId="TestWs01" segmentId={testId(42)} opened onClose={closed} />,
  )

  await expect
    .element(screen.getByText('The contacts this segment matches now', { exact: false }))
    .toBeVisible()
  await screen.getByRole('checkbox', { name: 'Photo', exact: true }).click()
  await screen.getByRole('button', { name: 'Export', exact: true }).click()

  await expect.poll(() => sent[0]?.segmentId).toBe(testId(42))
  expect(sent[0]?.columns).not.toContain('avatar')
  await expect
    .element(screen.getByText('You need the contacts:export permission to export contacts.'))
    .toBeVisible()
})
