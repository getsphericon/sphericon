import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource, SiteAutomationStep } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { emailIntegration, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })

const STEP: SiteAutomationStep = { type: 'telegram', text: 'Hi <b>there</b>', integrationId: '31' }

const INTEGRATIONS = [
  emailIntegration('1', 'Primary SMTP', { isDefault: true }),
  emailIntegration('31', 'Acme bot', {
    channel: 'telegram',
    provider: 'telegram',
    config: { kind: 'telegram', botId: 7001, username: 'acme_bot' },
  }),
]

function stored(steps: SiteAutomationStep[]): SiteAutomationResource {
  return {
    id: '5',
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: { number: 1, triggerEvent: 'telegram.bound', exitEvents: [], goalEvent: null, steps },
    ...TIMESTAMPS,
  }
}

function flowNode(index: number) {
  const node = document.querySelectorAll('.react-flow__node')[index]
  if (!node) {
    throw new Error('the canvas has not drawn the node yet')
  }
  return node
}

test('a Telegram step round-trips: its bot and text load, an edit saves them back', async () => {
  const bodies: { steps?: SiteAutomationStep[] }[] = []
  let current = stored([STEP])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = await request.json()
      bodies.push(body)
      current = stored(body.steps ?? [STEP])
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.poll(() => flowNode(1)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(1)).click()
  await screen.getByText('Properties').click()
  const picker = screen.getByRole('combobox', { name: 'Integration' })
  await expect.element(picker).toHaveValue('Acme bot')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Primary SMTP' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Acme bot' }).click()
  const text = screen.getByRole('textbox', { name: 'Message' })
  await expect.element(text).toHaveValue('Hi <b>there</b>')
  await expect.element(screen.getByText('15 / 4096')).toBeVisible()
  await text.fill('Welcome')
  await expect.element(screen.getByText('7 / 4096')).toBeVisible()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([[{ type: 'telegram', text: 'Welcome', integrationId: '31' }]])
})

test('a Telegram text over the limit is flagged by the counter', async () => {
  worker.use(
    handleSiteAutomationsGet(() =>
      HttpResponse.json(stored([{ type: 'telegram', text: 'x'.repeat(4100) }])),
    ),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.poll(() => flowNode(1)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(1)).click()
  await screen.getByText('Properties').click()

  await expect.element(screen.getByText('4100 / 4096')).toBeVisible()
  await expect.element(screen.getByText(/refused until the text is shorter/)).toBeVisible()
})
