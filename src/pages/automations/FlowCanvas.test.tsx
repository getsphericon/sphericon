import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteContactsList,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationStep,
  SiteAutomationResource,
  UpdateAutomationInput,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { stepId, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})
const UUID = /^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/

const WAIT: AutomationStep = { id: stepId(11), type: 'wait', seconds: 60 }
const TAG: AutomationStep = { id: stepId(12), type: 'apply_tag', tag: 'vip' }

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
      trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
      exitEvents: [],
      goalEvent: null,
      steps: { id: stepId(1), steps },
    },
    revision: 1,
    ...TIMESTAMPS,
  }
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(stored)
    }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteContactsList({
      body: { items: [], page: 1, pageSize: 50, totalItems: 0, totalPages: 0 },
    }),
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  const saved = () => bodies.map((body) => body.steps?.steps)
  return { screen, saved }
}

type Screen = Awaited<ReturnType<typeof editing>>['screen']

// The button of the canvas node whose kind is `label`, then what it says beside it.
function nodeButton(screen: Screen, label: string) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) })
}

// The top edge of an element on the page.
function topOf(locator: ReturnType<Screen['getByRole']>) {
  return locator.element().getBoundingClientRect().top
}

test('the canvas lays a linear flow out top to bottom, an insertion point under each node', async () => {
  const { screen } = await editing([WAIT, TAG])
  await expect.element(nodeButton(screen, 'Apply tag')).toBeVisible()

  const tops = [
    topOf(nodeButton(screen, 'Trigger')),
    topOf(screen.getByRole('button', { name: 'Add a step here' }).nth(0)),
    topOf(nodeButton(screen, 'Wait')),
    topOf(screen.getByRole('button', { name: 'Add a step here' }).nth(1)),
    topOf(nodeButton(screen, 'Apply tag')),
    topOf(screen.getByRole('button', { name: 'Add a step here' }).nth(2)),
  ]

  expect(tops.toSorted((a, b) => a - b)).toEqual(tops)
  expect(new Set(tops).size).toBe(tops.length)
})

test('inserting a Step on an edge puts it there, with an id of its own, edited in its panel', async () => {
  const { screen, saved } = await editing([WAIT, TAG])

  await screen.getByRole('button', { name: 'Add a step here' }).nth(1).click()
  await screen.getByRole('menuitem', { name: 'Remove tag' }).click()
  await screen.getByRole('textbox', { name: 'Tag' }).fill('lead')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(saved)
    .toEqual([[WAIT, { id: expect.stringMatching(UUID), type: 'remove_tag', tag: 'lead' }, TAG]])
})

test('removing a single Step needs no confirmation, and leaves the open panel open', async () => {
  const { screen, saved } = await editing([WAIT, TAG])
  await nodeButton(screen, 'Apply tag').click()

  await screen.getByRole('button', { name: 'Remove step' }).first().click()

  await expect.element(nodeButton(screen, 'Wait')).not.toBeInTheDocument()
  await expect.element(screen.getByRole('textbox', { name: 'Tag' })).toHaveValue('vip')
  await screen.getByRole('button', { name: 'Save' }).click()
  await expect.poll(saved).toEqual([[TAG]])
})

test('a copied Step follows its original with an id of its own', async () => {
  const { screen, saved } = await editing([WAIT, TAG])

  await nodeButton(screen, 'Apply tag').click()
  await screen.getByRole('button', { name: 'Duplicate step' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(saved)
    .toEqual([[WAIT, TAG, { id: expect.stringMatching(UUID), type: 'apply_tag', tag: 'vip' }]])
})

test('a wait and a tag step are edited in the side panel and saved', async () => {
  const { screen, saved } = await editing([WAIT, TAG])

  await nodeButton(screen, 'Wait').click()
  await screen.getByRole('textbox', { name: 'Wait (seconds)' }).fill('7200')
  await nodeButton(screen, 'Apply tag').click()
  await screen.getByRole('textbox', { name: 'Tag' }).fill('customer')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(saved).toEqual([
    [
      { ...WAIT, seconds: 7200 },
      { ...TAG, tag: 'customer' },
    ],
  ])
})

test("a Step's field is checked when it loses focus, and the Save stops on it", async () => {
  const { screen, saved } = await editing([TAG])

  await nodeButton(screen, 'Apply tag').click()
  const tag = screen.getByRole('textbox', { name: 'Tag' })
  await tag.fill('')
  await screen.getByRole('textbox', { name: 'Name' }).click()
  await expect.element(screen.getByText('This field is required')).toBeVisible()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(tag).toHaveFocus()
  expect(saved()).toEqual([])
})

test('an HTTP step and an email step inserted on the canvas are edited in their panels and saved', async () => {
  const { screen, saved } = await editing([])

  await screen.getByRole('button', { name: 'Add a step here' }).click()
  await screen.getByRole('menuitem', { name: 'Call a URL' }).click()
  await screen.getByRole('textbox', { name: 'URL' }).fill('https://hooks.example.com/x')
  await screen.getByRole('button', { name: 'Add a step here' }).last().click()
  await screen.getByRole('menuitem', { name: 'Send email' }).click()
  await screen.getByRole('textbox', { name: 'Subject' }).fill('Welcome')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(saved).toEqual([
    [
      {
        id: expect.stringMatching(UUID),
        type: 'http',
        body: '{}',
        headers: [],
        url: 'https://hooks.example.com/x',
      },
      { id: expect.stringMatching(UUID), type: 'email', subject: 'Welcome', body: '' },
    ],
  ])
})
