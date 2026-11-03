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

function canvasPane() {
  const pane = document.querySelector('.react-flow__pane')
  if (!pane) {
    throw new Error('the canvas has not drawn yet')
  }
  return pane
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
  await screen.getByText('Properties').click({ force: true })
  const picker = screen.getByRole('combobox', { name: 'Integration' })
  await expect.element(picker).toHaveValue('Acme bot')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Primary SMTP' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Acme bot' }).click()
  const text = screen.getByRole('textbox', { name: 'Message' })
  await expect.element(text).toHaveValue('Hi <b>there</b>')
  await expect.element(screen.getByText('15 / 4,096')).toBeVisible()
  await text.fill('Welcome')
  await expect.element(screen.getByText('7 / 4,096')).toBeVisible()
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
  await expect.element(screen.getByText('4,100 / 4,096')).toBeVisible()
  await expect.element(screen.getByText(/it cannot go out until it is shorter/)).toBeVisible()
})

test('adding an image switches the counter to the caption limit, and buttons and the preview switch round-trip', async () => {
  const bodies: { steps?: SiteAutomationStep[] }[] = []
  let current = stored([{ type: 'telegram', text: 'Hello {{ first_name }}', integrationId: '31' }])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = await request.json()
      bodies.push(body)
      current = stored(body.steps ?? [])
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
  await expect.element(screen.getByText('22 / 4,096')).toBeVisible()

  await screen.getByRole('textbox', { name: 'Image URL' }).fill('https://cdn.example.com/a.png')
  await expect.element(screen.getByText('22 / 1,024')).toBeVisible()
  const previewSwitch = screen.getByRole('switch', { name: 'Show a preview of links in the text' })
  await previewSwitch.click()
  await expect.element(previewSwitch).not.toBeChecked()
  await screen.getByRole('button', { name: 'Add a button' }).click()
  await screen.getByRole('textbox', { name: 'Button 1 label' }).fill('Open')
  await screen.getByRole('textbox', { name: 'Button 1 URL' }).fill('https://example.com/a')
  await expect.element(screen.getByTestId('telegram-preview')).toHaveTextContent('Hello AlexOpen')
  // The open properties panel covers the toolbar at this viewport: deselect the node first.
  await page.elementLocator(canvasPane()).click({ position: { x: 5, y: 300 } })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([
      [
        {
          type: 'telegram',
          text: 'Hello {{ first_name }}',
          integrationId: '31',
          imageUrl: 'https://cdn.example.com/a.png',
          buttons: [{ label: 'Open', url: 'https://example.com/a' }],
          linkPreview: false,
        },
      ],
    ])

  // Back from the server, the saved content loads into the editor again.
  await page.elementLocator(flowNode(1)).click()
  await expect.element(screen.getByRole('textbox', { name: 'Button 1 label' })).toHaveValue('Open')
})

test('the unsubscribe button of a Telegram step is switched on with a label and saved', async () => {
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
  await screen.getByText('Unsubscribe button', { exact: true }).click({ force: true })
  const label = screen.getByRole('textbox', { name: 'Button label' })
  await expect.element(label).toHaveValue('Unsubscribe')
  await label.fill('Stop messages')
  // The open properties panel covers the toolbar at this viewport: deselect the node first.
  await page.elementLocator(canvasPane()).click({ position: { x: 5, y: 300 } })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([[{ ...STEP, unsubscribeButton: 'Stop messages' }]])
})

test('a Telegram step with an unsubscribe button loads its label, and switching it off drops it', async () => {
  const bodies: { steps?: SiteAutomationStep[] }[] = []
  let current = stored([{ ...STEP, unsubscribeButton: 'Stop' }])
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
  await expect.element(screen.getByRole('textbox', { name: 'Button label' })).toHaveValue('Stop')
  await screen.getByText('Unsubscribe button', { exact: true }).click({ force: true })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies.map((body) => body.steps)).toEqual([[STEP]])
})
