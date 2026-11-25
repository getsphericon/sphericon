import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationPathInput,
  AutomationStep,
  SiteAutomationResource,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import {
  emailIntegration,
  stepId,
  TIMESTAMPS,
  testId,
  workspace as workspacePayload,
} from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

type Screen = Awaited<ReturnType<typeof renderWithRouter>>['screen']

// Opens the canvas node whose kind is `label` in the side panel; `index` picks among nodes of one
// kind, top to bottom.
async function openNode(screen: Screen, label: string, index = 0) {
  await screen
    .getByRole('button', { name: new RegExp(`^${label}`) })
    .nth(index)
    .click()
}

const STEP: AutomationStep = {
  id: stepId(11),
  type: 'sms',
  text: 'Hi {{ first_name }}',
  linkTracking: true,
  integrationId: testId(41),
}

// The editor's counter reads the Workspace's short-link length.
const WORKSPACE = {
  ...workspacePayload(),
  name: 'Test',
  postalAddress: '',
  mcpUrl: 'https://mcp.sphericon.test/',
}

const INTEGRATIONS = [
  emailIntegration(testId(1), 'Primary SMTP', { isDefault: true }),
  emailIntegration(testId(31), 'Acme bot', {
    channel: 'telegram',
    provider: 'telegram',
    config: { kind: 'telegram', botId: 7001, username: 'acme_bot' },
  }),
  emailIntegration(testId(41), 'Acme Twilio', {
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

function stored(steps: AutomationStep[]): SiteAutomationResource {
  return {
    id: testId(5),
    name: 'Cart reminder',
    status: 'inactive',
    live: null,
    draft: {
      number: 1,
      trigger: { kind: 'event', action: 'cart_abandoned', entryPolicy: 'once', cooldown: 3600 },
      exitEvents: [],
      goalEvent: null,
      steps: { id: stepId(1), steps },
    },
    revision: 1,
    ...TIMESTAMPS,
  }
}

test('an SMS step round-trips: its Integration and text load, an edit saves them back', async () => {
  const bodies: { steps?: AutomationPathInput }[] = []
  const current = stored([STEP])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
    handleSiteWorkspacesList({ body: [WORKSPACE] }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send SMS')
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
      {
        id: stepId(1),
        steps: [
          {
            id: stepId(11),
            type: 'sms',
            text: 'Your cart is waiting: {{ unsubscribe_url }}',
            linkTracking: false,
          },
        ],
      },
    ])
})
