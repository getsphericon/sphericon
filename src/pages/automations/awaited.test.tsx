import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page, userEvent } from 'vitest/browser'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsUnknownPath,
  handleSiteAutomationsUpdate,
  handleSiteContactsList,
  handleSiteCustomFieldsList,
  handleSiteEnrollmentsList,
  handleSiteEventsActions,
  handleSiteEventsPropertyKeys,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationStep,
  BranchCondition,
  SiteAutomationResource,
  UpdateAutomationInput,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import {
  NO_AUTOMATION_REPORT,
  page as pageOf,
  stepId,
  TIMESTAMPS,
  testId,
  workspace,
} from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

// The editor helps an author use the Event a Wait for event arrived on (ADR 0136): the merge tag
// picker and the Branch's Condition builder offer `awaited.<name>` for the Wait for events that can
// come before the Step, and a Step reading a name not every path before it passes is warned of.

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})

// A stored Wait for event on order.created, its `arrived` path holding the Steps.
function purchase(arrived: AutomationStep[] = []): AutomationStep {
  return {
    id: stepId(20),
    type: 'wait_for_event',
    name: 'purchase',
    timeoutSeconds: 86_400,
    awaits: { kind: 'event', matchers: [{ action: 'order.created' }] },
    arrived: { id: stepId(21), steps: arrived },
    timedOut: { id: stepId(22), steps: [] },
  }
}

// An SMS as stored; a save sends it with its link tracking left at the server's default.
function sms(id: number, text: string): AutomationStep {
  return { id: stepId(id), type: 'sms', text, linkTracking: true }
}

function sentSms(id: number, text: string) {
  return { id: stepId(id), type: 'sms', text }
}

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}

// A Branch on the Contact's Tag, its listed path holding the Steps given.
function branch(listed: AutomationStep[] = []): AutomationStep {
  return {
    id: stepId(40),
    type: 'branch',
    paths: [{ id: stepId(41), condition: VIP, steps: listed }],
    otherwise: { id: stepId(42), steps: [] },
  }
}

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

// Renders the editor over the stored steps, recording the body of every save.
async function editing(steps: AutomationStep[]) {
  const bodies: UpdateAutomationInput[] = []
  const stored: SiteAutomationResource = {
    id: testId(5),
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: {
      number: 1,
      trigger: { kind: 'event', action: 'signed_up', entryPolicy: 'once', cooldown: 3600 },
      exitEvents: [],
      goalEvent: null,
      steps: { id: stepId(1), steps },
    },
    revision: 1,
    ...TIMESTAMPS,
  }
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationReportsGet({ body: NO_AUTOMATION_REPORT }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(stored)
    }),
    handleSiteAutomationsUnknownPath({ body: { needed: false } }),
    handleSiteEventsActions({
      body: { actions: [], eventsKeptDays: 400, messageActions: [], keptActions: [] },
    }),
    handleSiteEventsPropertyKeys({ body: { keys: [] } }),
    handleSiteCustomFieldsList({ body: pageOf([]) }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteContactsList({ body: pageOf([]) }),
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteEnrollmentsList({ body: pageOf([]) }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  const saved = () => bodies.map((body) => body.steps?.steps)
  return { screen, saved }
}

type Screen = Awaited<ReturnType<typeof editing>>['screen']

// The button of the canvas node whose kind is `label`; `index` picks among nodes of one kind, in
// the canvas's order.
function nodeButton(screen: Screen, label: string, index = 0) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) }).nth(index)
}

function panelOf(screen: Screen, label: string) {
  return screen.getByRole('region', { name: label })
}

async function save(screen: Screen) {
  await screen.getByRole('button', { name: 'Save' }).click()
}

test('the merge tag picker offers awaited.purchase after the Wait for event purchase, and not before it', async () => {
  const { screen } = await editing([sms(10, 'Hi'), purchase([sms(30, 'Thanks')])])

  await nodeButton(screen, 'Send SMS', 0).click()
  await panelOf(screen, 'Send SMS').getByText('Insert a merge tag').click()
  await expect.element(screen.getByRole('menuitem', { name: 'event.action' })).toBeVisible()
  await expect
    .element(screen.getByRole('menuitem', { name: 'awaited.purchase.action' }))
    .not.toBeInTheDocument()
  await userEvent.keyboard('{Escape}')

  await nodeButton(screen, 'Send SMS', 1).click()
  await panelOf(screen, 'Send SMS').getByText('Insert a merge tag').click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'awaited.purchase.action' }))
    .toBeVisible()
})

test("a property of the awaited Event is inserted by the Wait for event's name and saved", async () => {
  const { screen, saved } = await editing([purchase([sms(30, '')])])

  await nodeButton(screen, 'Send SMS').click()
  const panel = panelOf(screen, 'Send SMS')
  await panel.getByRole('button', { name: 'Insert event property' }).click()
  await screen.getByLabelText('Event', { exact: true }).first().click()
  await screen.getByRole('option', { name: 'awaited.purchase' }).click()
  await screen.getByRole('textbox', { name: 'Property name' }).fill('product')
  await screen.getByRole('button', { name: 'Insert', exact: true }).click()
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        ...purchase(),
        arrived: {
          id: stepId(21),
          steps: [sentSms(30, '{{ awaited.purchase.properties.product }}')],
        },
      },
    ],
  ])
})

test('a Step using awaited.purchase after a Branch path that skips the wait is warned of, and saves', async () => {
  const text = 'Your {{ awaited.purchase.properties.product }}'
  const { screen, saved } = await editing([branch([purchase()]), sms(50, text)])

  await expect.element(screen.getByText('Not every path passes awaited.purchase')).toBeVisible()
  await nodeButton(screen, 'Send SMS').click()
  await expect
    .element(
      panelOf(screen, 'Send SMS').getByText(
        'Not every path to this step passes the step that keeps awaited.purchase.',
        { exact: false },
      ),
    )
    .toBeVisible()
  await save(screen)

  await expect.poll(saved).toEqual([[branch([purchase()]), sentSms(50, text)]])
})

test('a Step using awaited.purchase after the wait on every path is not warned of', async () => {
  const { screen } = await editing([
    purchase([sms(30, '{{ awaited.purchase.action }}')]),
    sms(50, 'Your {{ awaited.purchase.properties.product }}'),
  ])

  await nodeButton(screen, 'Send SMS', 1).click()
  await expect.element(panelOf(screen, 'Send SMS')).toBeVisible()
  await expect.element(screen.getByText('Not every path', { exact: false })).not.toBeInTheDocument()
})

test("the Branch's builder offers a test of the awaited Event by the Wait for event's name, and saves it", async () => {
  const { screen, saved } = await editing([purchase(), branch()])

  await nodeButton(screen, 'Branch').click()
  const panel = panelOf(screen, 'Branch')
  await panel.getByRole('button', { name: /Rule/ }).first().click()
  await panel.getByTestId('fields').last().click()
  await screen.getByRole('option', { name: 'Property of awaited.purchase' }).click()
  await panel.getByLabelText('Event property name').last().fill('plan')
  await panel.getByTestId('value-editor').last().fill('gold')
  await save(screen)

  const tested: BranchCondition = {
    ...VIP,
    entries: [
      ...VIP.entries,
      {
        kind: 'property',
        of: { kind: 'awaited', name: 'purchase' },
        key: 'plan',
        test: { kind: 'text', operator: 'eq', value: 'gold' },
      },
    ],
  }
  await expect.poll(saved).toEqual([
    [
      purchase(),
      {
        ...branch(),
        paths: [{ id: stepId(41), condition: tested, steps: [] }],
      },
    ],
  ])
})
