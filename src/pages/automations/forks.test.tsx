import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsUnknownPath,
  handleSiteAutomationsUpdate,
  handleSiteContactsList,
  handleSiteCustomFieldsList,
  handleSiteEnrollmentsList,
  handleSiteEventsActions,
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
import { page as pageOf, stepId, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

// The canvas edits forks (ADR 0124): Branches and their paths, Splits and their weights, End.

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})
const UUID = /^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/
const anyId = expect.stringMatching(UUID)

const TAG: AutomationStep = { id: stepId(12), type: 'apply_tag', tag: 'vip' }
const WAIT: AutomationStep = { id: stepId(13), type: 'wait', seconds: 60 }

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}
const OPENED: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'event', action: 'email.opened', operator: 'atLeast', times: 1 }],
}

// A property test of the Event the Enrollment began with, as the builder writes it.
function planIs(value: string): BranchCondition {
  return {
    kind: 'group',
    combinator: 'and',
    entries: [
      {
        kind: 'property',
        of: { kind: 'trigger' },
        key: 'plan',
        test: { kind: 'text', operator: 'eq', value },
      },
    ],
  }
}

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

// Renders the editor over the stored steps, recording the body of every save. The server says a
// Branch needs an unknown path when one of its Conditions counts an open.
async function editing(steps: AutomationStep[]) {
  const bodies: UpdateAutomationInput[] = []
  const stored: SiteAutomationResource = {
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
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(stored)
    }),
    handleSiteAutomationsUnknownPath(async ({ request }) => {
      const { conditions } = await request.json()
      return HttpResponse.json({ needed: JSON.stringify(conditions).includes('email.opened') })
    }),
    handleSiteEventsActions({ body: { actions: ['email.opened'] } }),
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

// The button of the canvas node whose kind is `label`.
function nodeButton(screen: Screen, label: string) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) }).first()
}

// Gives the nth path's Condition (0 first) the property test plan = value, in the builder.
async function setPlan(screen: Screen, nth: number, value: string) {
  const panel = screen.getByRole('region', { name: 'Branch' })
  await panel.getByRole('button', { name: /Rule/ }).nth(nth).click()
  await panel.getByTestId('fields').nth(nth).click()
  await screen.getByRole('option', { name: 'Event property' }).click()
  await panel.getByLabelText('Event property name').nth(nth).fill('plan')
  await panel.getByTestId('value-editor').nth(nth).fill(value)
}

async function save(screen: Screen) {
  await screen.getByRole('button', { name: 'Save' }).click()
}

test('a Branch added on the canvas, its paths added and reordered, saves the tree in that order', async () => {
  const { screen, saved } = await editing([TAG])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Branch' }).click()
  await setPlan(screen, 0, 'pro')
  await screen.getByRole('button', { name: 'Add path' }).last().click()
  await setPlan(screen, 1, 'free')
  await screen.getByRole('button', { name: 'Move path up' }).nth(1).click()
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        id: anyId,
        type: 'branch',
        paths: [
          { id: anyId, condition: planIs('free'), steps: [] },
          { id: anyId, condition: planIs('pro'), steps: [] },
        ],
        otherwise: { id: anyId, steps: [] },
      },
      TAG,
    ],
  ])
})

test("a Step inserted into a fork's empty path goes there", async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: VIP, steps: [WAIT] }],
    otherwise: { id: stepId(22), steps: [] },
  }
  const { screen, saved } = await editing([branch, TAG])
  await expect.element(screen.getByRole('button', { name: 'Otherwise' })).toBeVisible()

  // The places in the canvas's order: the root's first, path 1's two, then `otherwise`'s one.
  await screen.getByRole('button', { name: 'Add a step here' }).nth(3).click()
  await screen.getByRole('menuitem', { name: 'End' }).click()
  await save(screen)

  await expect
    .poll(saved)
    .toEqual([
      [{ ...branch, otherwise: { id: stepId(22), steps: [{ id: anyId, type: 'end' }] } }, TAG],
    ])
})

test('an End is offered only at the end of a path', async () => {
  const { screen } = await editing([WAIT])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Wait' })).toBeVisible()
  await expect.element(screen.getByRole('menuitem', { name: 'End' })).not.toBeInTheDocument()
})

test('removing a Branch whose paths hold Steps asks first, naming how many go with it', async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: VIP, steps: [WAIT, { ...TAG, id: stepId(14) }] }],
    otherwise: { id: stepId(22), steps: [] },
  }
  const { screen, saved } = await editing([branch, TAG])

  await screen.getByRole('button', { name: 'Remove step' }).first().click()
  await expect
    .element(screen.getByText('Removing this Branch also removes the 2 steps it holds.'))
    .toBeVisible()
  await screen.getByRole('button', { name: 'Remove', exact: true }).click()
  await save(screen)

  await expect.poll(saved).toEqual([[TAG]])
})

test('removing an empty path asks nothing', async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [
      { id: stepId(21), condition: VIP, steps: [WAIT] },
      { id: stepId(23), condition: VIP, steps: [] },
    ],
    otherwise: { id: stepId(22), steps: [] },
  }
  const { screen, saved } = await editing([branch])

  await nodeButton(screen, 'Branch').click()
  await screen.getByRole('button', { name: 'Remove path' }).nth(1).click()
  await save(screen)

  await expect.poll(saved).toEqual([[{ ...branch, paths: [branch.paths[0]] }]])
  await expect.element(screen.getByRole('dialog', { name: 'Remove path' })).not.toBeInTheDocument()
})

test('a Split whose weights do not sum to 100 shows the sum, on its node too, and is not saved', async () => {
  const split: AutomationStep = {
    id: stepId(20),
    type: 'split',
    paths: [
      { id: stepId(21), weight: 50, steps: [WAIT] },
      { id: stepId(22), weight: 50, steps: [] },
    ],
  }
  const { screen, saved } = await editing([split])

  await nodeButton(screen, 'Split').click()
  await screen.getByRole('textbox', { name: 'Path 1 weight' }).fill('30')
  await expect.element(screen.getByText('Total: 80%')).toBeVisible()
  await save(screen)

  await expect.element(screen.getByText('The weights sum to 80%, not 100%').first()).toBeVisible()
  // On the Split's node, and in its panel.
  expect(screen.getByText('The weights sum to 80%, not 100%').elements()).toHaveLength(2)
  await screen.getByRole('textbox', { name: 'Path 2 weight' }).fill('70')
  await save(screen)
  await expect.poll(saved).toEqual([
    [
      {
        ...split,
        paths: [
          { ...split.paths[0], weight: 30 },
          { ...split.paths[1], weight: 70 },
        ],
      },
    ],
  ])
})

test("a Branch path's Condition tests a property of the Event the Enrollment began with", async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: VIP, steps: [] }],
    otherwise: { id: stepId(22), steps: [] },
  }
  const { screen, saved } = await editing([branch])

  await nodeButton(screen, 'Branch').click()
  const panel = screen.getByRole('region', { name: 'Branch' })
  await panel.getByRole('button', { name: /Rule/ }).first().click()
  await panel.getByTestId('fields').last().click()
  await screen.getByRole('option', { name: 'Event property' }).click()
  await panel.getByLabelText('Event property name').fill('plan')
  await panel.getByTestId('value-editor').last().fill('pro')
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        ...branch,
        paths: [
          {
            id: stepId(21),
            condition: { ...VIP, entries: [...VIP.entries, ...planIs('pro').entries] },
            steps: [],
          },
        ],
      },
    ],
  ])
})

test("a fork's error shows on its node and in its open panel, and stops the Save", async () => {
  const { screen, saved } = await editing([])

  await screen.getByRole('button', { name: 'Add a step here' }).click()
  await screen.getByRole('menuitem', { name: 'Branch' }).click()
  await save(screen)

  await expect.element(screen.getByText('Path 1 needs a condition')).toBeVisible()
  await expect
    .element(screen.getByRole('region', { name: 'Branch' }).getByText('This field is required'))
    .toBeVisible()
  expect(saved()).toEqual([])
})

test('a Save stopped on a fork whose panel is closed opens it, showing the error', async () => {
  const { screen, saved } = await editing([])

  await screen.getByRole('button', { name: 'Add a step here' }).click()
  await screen.getByRole('menuitem', { name: 'Branch' }).click()
  await screen.getByRole('button', { name: 'Close panel' }).click()
  await save(screen)

  await expect.element(screen.getByText('Path 1 needs a condition')).toBeVisible()
  await expect
    .element(screen.getByRole('region', { name: 'Branch' }).getByText('This field is required'))
    .toBeVisible()
  expect(saved()).toEqual([])
})

test('an open test added to a Condition adds the unknown path', async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: VIP, steps: [] }],
    otherwise: { id: stepId(22), steps: [] },
  }
  const { screen, saved } = await editing([branch])

  await nodeButton(screen, 'Branch').click()
  const panel = screen.getByRole('region', { name: 'Branch' })
  await panel.getByRole('button', { name: /Rule/ }).first().click()
  await panel.getByTestId('fields').last().click()
  await screen.getByRole('option', { name: 'Opened an email' }).click()
  await expect.element(screen.getByRole('button', { name: 'Unknown' })).toBeVisible()
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        ...branch,
        paths: [
          {
            id: stepId(21),
            condition: { ...VIP, entries: [...VIP.entries, ...OPENED.entries] },
            steps: [],
          },
        ],
        unknown: { id: anyId, steps: [] },
      },
    ],
  ])
})

test('removing the last open test removes the unknown path, asking first while it holds Steps', async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [
      {
        id: stepId(21),
        condition: { ...VIP, entries: [...VIP.entries, ...OPENED.entries] },
        steps: [],
      },
    ],
    otherwise: { id: stepId(22), steps: [] },
    unknown: { id: stepId(23), steps: [WAIT] },
  }
  const { screen, saved } = await editing([branch])

  await nodeButton(screen, 'Branch').click()
  const panel = screen.getByRole('region', { name: 'Branch' })
  await panel.getByTitle('Remove rule').last().click()
  await expect
    .element(screen.getByText('Removing it also removes the step it holds.', { exact: false }))
    .toBeVisible()
  await screen.getByRole('button', { name: 'Remove', exact: true }).click()
  await expect.element(screen.getByRole('button', { name: 'Unknown' })).not.toBeInTheDocument()
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        id: stepId(20),
        type: 'branch',
        paths: [{ id: stepId(21), condition: VIP, steps: [] }],
        otherwise: branch.otherwise,
      },
    ],
  ])
})

// Publish arms a Branch by the same rule the editor follows; when it refuses one (an unknown path
// missing or not needed), the editor shows the refusal, which names the Step.
test("Publish's refusal of a Branch's unknown path shows in the editor", async () => {
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: VIP, steps: [] }],
    otherwise: { id: stepId(22), steps: [] },
  }
  const refusal = `step ${stepId(20)} (branch): a branch whose conditions count an open or a click needs an unknown path`
  const { screen } = await editing([branch])
  worker.use(handleSiteAutomationsPublish(() => problem(422, { detail: refusal })))

  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.element(screen.getByText(refusal)).toBeVisible()
})

// A count cleared to be typed again leaves the Condition incomplete for a moment: the unknown path
// stays as it is meanwhile, its Steps and id with it, and nothing is asked.
test('retyping the count of an open test keeps the unknown path as it is', async () => {
  const openedTwice: BranchCondition = {
    kind: 'group',
    combinator: 'and',
    entries: [{ kind: 'event', action: 'email.opened', operator: 'atLeast', times: 2 }],
  }
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition: openedTwice, steps: [] }],
    otherwise: { id: stepId(22), steps: [] },
    unknown: { id: stepId(23), steps: [WAIT] },
  }
  const { screen, saved } = await editing([branch])

  await nodeButton(screen, 'Branch').click()
  const times = screen.getByRole('region', { name: 'Branch' }).getByLabelText('Times')
  await times.fill('')
  await expect.element(screen.getByRole('button', { name: 'Unknown' })).toBeVisible()
  await times.fill('3')
  await save(screen)

  await expect.poll(saved).toEqual([
    [
      {
        ...branch,
        paths: [
          {
            id: stepId(21),
            condition: { ...openedTwice, entries: [{ ...openedTwice.entries[0], times: 3 }] },
            steps: [],
          },
        ],
      },
    ],
  ])
  expect(screen.getByText('needs no unknown path', { exact: false }).elements()).toEqual([])
})

test('an End offers no copy: a copy after it would never run', async () => {
  const { screen } = await editing([WAIT, { id: stepId(14), type: 'end' }])

  await nodeButton(screen, 'End').click()

  await expect.element(screen.getByRole('region', { name: 'End' })).toBeVisible()
  await expect
    .element(screen.getByRole('button', { name: 'Duplicate step' }))
    .not.toBeInTheDocument()
})
