import { useState } from 'react'
import { beforeEach, expect, test } from 'vitest'

import { handleSiteWorkspacesList } from '../../generated/site/msw.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import type { SmsContent } from './SmsContentEditor.tsx'
import { SmsContentEditor } from './SmsContentEditor.tsx'

// The Workspace's short tracked links are 30 characters long.
beforeEach(() => {
  worker.use(
    handleSiteWorkspacesList({
      body: [
        {
          id: 'AcmeWs01',
          name: 'Acme',
          collectKey: 'omck_test_key',
          postalAddress: '',
          timezone: 'UTC',
          accountLinkAvailable: false,
          permissions: [],
          mcpUrl: 'https://mcp.sphericon.test/',
          trackerUrl: 'https://t.sphericon.test',
          shortLinkLength: 30,
          holds: [],
          role: 'owner',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    }),
  )
})

function Harness({ initial }: { initial: SmsContent }) {
  const [value, setValue] = useState(initial)
  return <SmsContentEditor value={value} onChange={setValue} />
}

const render = async (text: string) =>
  await renderWithRouter(
    <Harness initial={{ text, linkTracking: true }} />,
    routeMount(workspaceRoute, { workspaceId: 'AcmeWs01' }),
  )

test('typing a Cyrillic character switches the counter to UCS-2', async () => {
  const { screen } = await render('')
  const text = screen.getByRole('textbox', { name: 'Message' })

  await text.fill('Hi there')
  await expect.element(screen.getByText('8 characters · GSM-7 · 1 segment')).toBeVisible()

  await text.fill('Hi there, Борис')

  await expect.element(screen.getByText('15 characters · UCS-2 · 1 segment')).toBeVisible()
})

test('a text past one part counts its segments, an extension character as two', async () => {
  const { screen } = await render(`${'a'.repeat(159)}€`)

  await expect.element(screen.getByText('161 characters · GSM-7 · 2 segments')).toBeVisible()
})

test('a tracked link counts as the short link it goes out as, and as written once tracking is off', async () => {
  const { screen } = await render(`Shop https://shop.example/${'p'.repeat(100)}`)

  await expect.element(screen.getByText('35 characters · GSM-7 · 1 segment')).toBeVisible()

  await screen.getByText('Track links').click({ force: true })

  await expect.element(screen.getByText('126 characters · GSM-7 · 1 segment')).toBeVisible()
})
