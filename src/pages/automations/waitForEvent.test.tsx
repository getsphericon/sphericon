import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
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
  AutomationPath,
  AutomationStep,
  SiteAutomationResource,
  UpdateAutomationInput,
  WaitForEventCondition,
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

// The canvas edits a Wait for event on an Event (ADR 0124): a fork with its `arrived` and
// `timedOut` paths, whose panel holds its name (ADR 0136), timeout, awaited Events (ADR 0180) and
// Condition.

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})
const UUID = /^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/
const anyId = expect.stringMatching(UUID)

const TAG: AutomationStep = { id: stepId(12), type: 'apply_tag', tag: 'vip' }
const WAIT: AutomationStep = { id: stepId(13), type: 'wait', seconds: 60 }

// A property test of the Event the Enrollment began with, as the builder writes it.
const PRO: WaitForEventCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [
    {
      kind: 'property',
      of: { kind: 'trigger' },
      key: 'plan',
      test: { kind: 'text', operator: 'eq', value: 'pro' },
    },
  ],
}

// A stored Wait for event on order.created, a day long, its `arrived` path holding the Steps.
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

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

// Renders the editor over the stored steps, recording the body of every save. The server stores
// what a save sends, as a stored version reads it back; `hold` keeps a save's answer back until it
// resolves.
async function editing(steps: AutomationStep[], hold?: Promise<unknown>) {
  const bodies: UpdateAutomationInput[] = []
  const received = Promise.withResolvers<undefined>()
  let stored: SiteAutomationResource = {
    id: testId(5),
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: {
      number: 1,
      trigger: { kind: 'event', action: 'order.paid', entryPolicy: 'once', cooldown: 3600 },
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
      const body = await request.json()
      bodies.push(body)
      received.resolve(undefined)
      await hold
      // Every Step and path is sent with its id, so the stored version reads as it was sent.
      const saved: AutomationPath = JSON.parse(JSON.stringify(body.steps))
      stored = {
        ...stored,
        revision: stored.revision + 1,
        ...(stored.draft === null ? {} : { draft: { ...stored.draft, steps: saved } }),
      }
      return HttpResponse.json(stored)
    }),
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
  return { screen, saved, received: received.promise }
}

type Screen = Awaited<ReturnType<typeof editing>>['screen']

// The button of the canvas node whose kind is `label`.
function nodeButton(screen: Screen, label: string) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) }).first()
}

function panelOf(screen: Screen) {
  return screen.getByRole('region', { name: 'Wait for event' })
}

async function save(screen: Screen) {
  await screen.getByRole('button', { name: 'Save' }).click()
}

test('a Wait for event inserted on the canvas, its Event, timeout and Condition set, saves the Step', async () => {
  const { screen, saved } = await editing([TAG])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait for event' }).click()
  const panel = panelOf(screen)
  await panel.getByRole('textbox', { name: 'Name' }).fill('purchase')
  await panel.getByRole('textbox', { name: 'Timeout (seconds)' }).fill('259200')
  await panel.getByRole('combobox', { name: 'Event' }).fill('order.created')
  await panel.getByRole('button', { name: /Rule/ }).last().click()
  await panel.getByTestId('fields').last().click()
  await screen.getByRole('option', { name: 'Event property' }).click()
  await panel.getByLabelText('Event property name').last().fill('plan')
  await panel.getByTestId('value-editor').last().fill('pro')
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        id: anyId,
        type: 'wait_for_event',
        name: 'purchase',
        timeoutSeconds: 259_200,
        awaits: { kind: 'event', matchers: [{ action: 'order.created' }], condition: PRO },
        arrived: { id: anyId, steps: [] },
        timedOut: { id: anyId, steps: [] },
      },
      TAG,
    ],
  ])
})

test('the name is locked once the Wait for event is first saved, with the reason', async () => {
  const { screen, saved } = await editing([])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait for event' }).click()
  const panel = panelOf(screen)
  await expect.element(panel.getByRole('textbox', { name: 'Name' })).toBeEnabled()
  await panel.getByRole('textbox', { name: 'Name' }).fill('purchase')
  await panel.getByRole('combobox', { name: 'Event' }).fill('order.created')
  await save(screen)
  await expect.poll(saved).toHaveLength(1)

  await expect.element(panel.getByRole('textbox', { name: 'Name' })).toBeDisabled()
  await expect
    .element(panel.getByText('The name is fixed once the step is saved', { exact: false }))
    .toBeVisible()
})

// A name typed while the first Save is pending is no name the server stored: it stays editable, so
// it can be put back, and locks once it is the stored one again.
test('a name changed while the first save is pending stays editable until it is the stored one', async () => {
  const release = Promise.withResolvers<undefined>()
  const { screen, received } = await editing([], release.promise)

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait for event' }).click()
  const panel = panelOf(screen)
  const name = panel.getByRole('textbox', { name: 'Name' })
  await name.fill('purchase')
  await panel.getByRole('combobox', { name: 'Event' }).fill('order.created')
  await save(screen)
  await received
  await name.fill('order')
  release.resolve(undefined)

  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
  await expect.element(name).toBeEnabled()
  await name.fill('purchase')
  await expect.element(name).toBeDisabled()
})

test('a missing timeout shows on the node and in the panel, and nothing is saved', async () => {
  const { screen, saved } = await editing([purchase()])

  await nodeButton(screen, 'Wait for event').click()
  await panelOf(screen).getByRole('textbox', { name: 'Timeout (seconds)' }).clear()
  await save(screen)

  // On the node, and under the field in the panel.
  await expect.element(screen.getByText('Timeout (seconds) is required.')).toBeVisible()
  await expect.element(panelOf(screen).getByText('This field is required')).toBeVisible()
  expect(saved()).toEqual([])
})

test('removing a Wait for event whose paths hold Steps asks first, naming how many go with it', async () => {
  const { screen, saved } = await editing([purchase([WAIT, { ...TAG, id: stepId(14) }]), TAG])

  await screen.getByRole('button', { name: 'Remove step' }).first().click()
  await expect
    .element(screen.getByText('Removing this Wait for event also removes the 2 steps it holds.'))
    .toBeVisible()
  await screen.getByRole('button', { name: 'Remove', exact: true }).click()
  await save(screen)

  await expect.poll(saved).toEqual([[TAG]])
})

test('an edit made while saving stays unsaved after the save, with the Steps put in its paths', async () => {
  const release = Promise.withResolvers<undefined>()
  const { screen, saved, received } = await editing([purchase()], release.promise)

  await nodeButton(screen, 'Wait for event').click()
  await panelOf(screen).getByRole('textbox', { name: 'Timeout (seconds)' }).fill('3600')
  await save(screen)
  await received
  // The places in the canvas's order: the root's first, `arrived`'s, `timedOut`'s, the root's last.
  await screen.getByRole('button', { name: 'Add a step here' }).nth(1).click()
  await screen.getByRole('menuitem', { name: 'Apply tag' }).click()
  await nodeButton(screen, 'Wait for event').click()
  await panelOf(screen).getByRole('textbox', { name: 'Timeout (seconds)' }).fill('7200')
  release.resolve(undefined)

  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await expect
    .element(panelOf(screen).getByRole('textbox', { name: 'Timeout (seconds)' }))
    .toHaveValue('7,200')
  await nodeButton(screen, 'Apply tag').click()
  await screen.getByRole('textbox', { name: 'Tag' }).fill('buyer')
  await save(screen)

  const sent = purchase()
  await expect.poll(saved).toEqual([
    [{ ...sent, timeoutSeconds: 3600 }],
    [
      {
        ...sent,
        timeoutSeconds: 7200,
        arrived: { id: stepId(21), steps: [{ id: anyId, type: 'apply_tag', tag: 'buyer' }] },
      },
    ],
  ])
})
