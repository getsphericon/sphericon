import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsRevert,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { Route } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(Route, { workspaceId: 'TestWs01', automationId: '5' })
const LIVE: SiteAutomationVersion = {
  number: 1,
  triggerEvent: 'email.clicked',
  exitEvents: [],
  goalEvent: null,
  steps: [],
}
const DRAFT: SiteAutomationVersion = { ...LIVE, number: 2 }
const STORED: SiteAutomationResource = {
  id: '5',
  name: 'Welcome flow',
  status: 'active',
  draft: null,
  live: LIVE,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function noEnrollmentsOrIntegrations() {
  worker.use(
    handleSiteIntegrationsList({ body: [] }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
}

function triggerNode() {
  const node = document.querySelector('[data-testid="rf__node-trigger"]')
  if (!node) {
    throw new Error('Trigger node has not rendered')
  }
  return node
}

test.each([
  { label: 'the first draft', draft: null },
  { label: 'an existing draft', draft: DRAFT },
])('saving $label preserves newer edits and leaves them unpublished', async ({ draft }) => {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  let stored = { ...STORED, draft }
  noEnrollmentsOrIntegrations()
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async () => {
      received.resolve(undefined)
      await release.promise
      stored = {
        ...STORED,
        name: 'Submitted name',
        draft: { ...DRAFT, triggerEvent: 'course_completed' },
      }
      return HttpResponse.json(stored)
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.poll(() => triggerNode()).toBeInstanceOf(Element)
  await page.elementLocator(triggerNode()).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('course_completed')
  await screen.getByLabelText('Name', { exact: false }).fill('Submitted name')
  await screen.getByRole('button', { name: 'Save' }).click()
  await received.promise
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeEnabled()
  await screen.getByLabelText('Name', { exact: false }).fill('Newer local name')
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('newer_event')
  release.resolve(undefined)
  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
  await expect
    .element(screen.getByLabelText('Name', { exact: false }))
    .toHaveValue('Newer local name')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger Event' }))
    .toHaveValue('newer_event')
  await screen.getByLabelText('Name', { exact: false }).fill('Submitted name')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('course_completed')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
})

test('a refused save preserves newer edits and the previous acknowledged content', async () => {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  noEnrollmentsOrIntegrations()
  worker.use(
    handleSiteAutomationsGet({ body: { ...STORED, draft: DRAFT } }),
    handleSiteAutomationsUpdate(async () => {
      received.resolve(undefined)
      await release.promise
      return problem(422, 'The definition was refused')
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByLabelText('Name', { exact: false }).fill('Submitted name')
  await screen.getByRole('button', { name: 'Save' }).click()
  await received.promise
  await expect.element(screen.getByRole('button', { name: 'Revert' })).toBeDisabled()
  await screen.getByLabelText('Name', { exact: false }).fill('Newer local name')
  release.resolve(undefined)
  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
  await expect
    .element(screen.getByLabelText('Name', { exact: false }))
    .toHaveValue('Newer local name')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await screen.getByLabelText('Name', { exact: false }).fill('Welcome flow')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
})

test('Revert replaces local content with live and prevents editing during the operation', async () => {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  noEnrollmentsOrIntegrations()
  worker.use(
    handleSiteAutomationsGet({
      body: { ...STORED, draft: { ...DRAFT, triggerEvent: 'course_completed' } },
    }),
    handleSiteAutomationsRevert(async () => {
      received.resolve(undefined)
      await release.promise
      return HttpResponse.json(STORED)
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.poll(() => triggerNode()).toBeInstanceOf(Element)
  await page.elementLocator(triggerNode()).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('unsaved_event')
  await screen.getByLabelText('Name', { exact: false }).fill('Unsaved local name')
  await screen.getByRole('button', { name: 'Revert' }).click()
  const confirmation = screen.getByRole('dialog', { name: 'Revert without saving?' })
  await expect
    .element(confirmation.getByText('Your changes to this automation are not saved.'))
    .toBeInTheDocument()
  await confirmation.getByRole('button', { name: 'Revert' }).click()
  await received.promise
  try {
    await expect.element(screen.getByLabelText('Name', { exact: false })).toBeDisabled()
    await expect.element(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
    await expect.element(screen.getByRole('combobox', { name: 'Trigger Event' })).toBeDisabled()
  } finally {
    release.resolve(undefined)
  }
  await expect.element(screen.getByText('Unpublished changes reverted')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Welcome flow')
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
  await page.elementLocator(triggerNode()).click()
  await screen.getByText('Properties').click()
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger Event' }))
    .toHaveValue('email.clicked')
  await expect.element(screen.getByRole('combobox', { name: 'Trigger Event' })).toBeEnabled()
})

test('Publish serializes operations and preserves the current canvas', async () => {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  const draft = { ...DRAFT, triggerEvent: 'course_completed' }
  noEnrollmentsOrIntegrations()
  worker.use(
    handleSiteAutomationsGet({ body: { ...STORED, draft } }),
    handleSiteAutomationsPublish(async () => {
      received.resolve(undefined)
      await release.promise
      return HttpResponse.json({ ...STORED, live: draft, draft: null })
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.poll(() => triggerNode()).toBeInstanceOf(Element)
  await page.elementLocator(triggerNode()).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('button', { name: 'Publish' }).click()
  await received.promise
  try {
    await expect.element(screen.getByLabelText('Name', { exact: false })).toBeDisabled()
    await expect.element(screen.getByRole('combobox', { name: 'Trigger Event' })).toBeDisabled()
    await expect.element(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    await expect.element(screen.getByRole('button', { name: 'Revert' })).toBeDisabled()
  } finally {
    release.resolve(undefined)
  }
  await expect.element(screen.getByText('Automation published')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger Event' }))
    .toHaveValue('course_completed')
  await expect.element(screen.getByRole('combobox', { name: 'Trigger Event' })).toBeEnabled()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
})

// What a pending Save carries is not lost by leaving (the write outlives the editor): only an
// edit made since asks first.
// The editor with a Save of a new name pending, whose write is never answered.
async function savingName() {
  const received = Promise.withResolvers<undefined>()
  noEnrollmentsOrIntegrations()
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async () => {
      received.resolve(undefined)
      return Promise.withResolvers<never>().promise
    }),
  )
  const rendered = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await rendered.screen.getByLabelText('Name', { exact: false }).fill('Submitted name')
  await rendered.screen.getByRole('button', { name: 'Save' }).click()
  await received.promise
  return rendered
}

test('leaving while Save is pending asks when the content changed since', async () => {
  const { screen, router } = await savingName()
  await screen.getByLabelText('Name', { exact: false }).fill('Newer name')
  router.history.push('/workspaces/TestWs01')

  await expect.element(screen.getByRole('dialog', { name: 'Leave without saving?' })).toBeVisible()
})

test('leaving while Save is pending does not ask when nothing changed since', async () => {
  const { screen, router } = await savingName()
  router.history.push('/workspaces/TestWs01')

  await expect.poll(() => router.history.location.pathname).toBe('/workspaces/TestWs01')
  expect(screen.getByRole('dialog', { name: 'Leave without saving?' }).elements()).toEqual([])
})
