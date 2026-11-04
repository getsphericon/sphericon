import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource, SiteAutomationStep } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { emailIntegration, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })

const STEP: SiteAutomationStep = { type: 'sms', text: 'Hi {{ first_name }}', integrationId: '41' }

// The editor's counter reads the Workspace's short-link length.
const WORKSPACE = {
  id: 'TestWs01',
  name: 'Test',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://mcp.sphericon.test/',
  trackerUrl: 'https://t.sphericon.test',
  shortLinkLength: 35,
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const INTEGRATIONS = [
  emailIntegration('1', 'Primary SMTP', { isDefault: true }),
  emailIntegration('31', 'Acme bot', {
    channel: 'telegram',
    provider: 'telegram',
    config: { kind: 'telegram', botId: 7001, username: 'acme_bot' },
  }),
  emailIntegration('41', 'Acme Twilio', {
    channel: 'sms',
    provider: 'twilio',
    config: {
      kind: 'twilio',
      accountSid: 'AC00000000000000000000000000000001',
      apiKeySid: null,
      from: '+15017250604',
      fromKind: 'number',
      callbacksSet: true,
    },
  }),
]

function stored(steps: SiteAutomationStep[]): SiteAutomationResource {
  return {
    id: '5',
    name: 'Cart reminder',
    status: 'inactive',
    live: null,
    draft: { number: 1, triggerEvent: 'cart_abandoned', exitEvents: [], goalEvent: null, steps },
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

test('an SMS step round-trips: its Integration and text load, an edit saves them back', async () => {
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
    handleSiteWorkspacesList({ body: [WORKSPACE] }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.poll(() => flowNode(1)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(1)).click()
  await screen.getByText('Properties').click({ force: true })
  const picker = screen.getByRole('combobox', { name: 'Integration' })
  await expect.element(picker).toHaveValue('Acme Twilio')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Acme bot' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('option', { name: 'Primary SMTP' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Workspace default' }).click()
  const text = screen.getByRole('textbox', { name: 'Message' })
  await expect.element(text).toHaveValue('Hi {{ first_name }}')
  await text.fill('Your cart is waiting: ')
  await screen.getByText('Insert a merge tag').click()
  await screen.getByRole('menuitem', { name: 'unsubscribe_url' }).click()
  await screen.getByText('Track links').click({ force: true })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([
      [{ type: 'sms', text: 'Your cart is waiting: {{ unsubscribe_url }}', linkTracking: false }],
    ])
})
