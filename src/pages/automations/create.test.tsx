import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAutomationsCreate } from '../../generated/site/msw.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$workspaceId/automations/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationCreatePage } from './create.tsx'

const WORKSPACE_ID = { workspaceId: 'TestWs01' }
const CREATE_ROUTE = routeMount(automationsCreateRoute, WORKSPACE_ID)
const NOW = '2026-01-01T00:00:00Z'

test('creating an automation sends the trimmed name and trigger, then opens the builder', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(
        {
          id: '7',
          name: 'Welcome',
          status: 'draft',
          triggerEvent: 'contact.created',
          steps: [],
          createdAt: NOW,
          updatedAt: NOW,
        },
        { status: 201 },
      )
    }),
  )

  const { screen, navigate } = await renderWithRouter(<AutomationCreatePage />, CREATE_ROUTE)
  await expect.element(screen.getByText('New automation')).toBeInTheDocument()

  await screen.getByLabelText('Name', { exact: false }).fill('  Welcome  ')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', triggerEvent: 'contact.created' }])
  await expect.element(screen.getByText('Automation created')).toBeInTheDocument()
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { workspaceId: 'TestWs01', automationId: '7' },
  })
})

test('the chosen trigger event is sent', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return problem(500)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationCreatePage />, CREATE_ROUTE)

  await screen.getByLabelText('Name', { exact: false }).fill('Opens')
  await screen.getByRole('combobox', { name: 'Trigger event' }).click()
  await screen.getByRole('option', { name: 'Email opened' }).click()
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Opens', triggerEvent: 'email.opened' }])
  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
})

test('Cancel links back to the list', async () => {
  const { screen } = await renderWithRouter(<AutomationCreatePage />, CREATE_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(automationsRoute, { workspaceId: 'TestWs01' }).initialPath)
})
