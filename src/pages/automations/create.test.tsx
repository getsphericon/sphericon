import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAutomationsCreate } from '../../generated/site/msw.gen.ts'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$slug/automations/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationCreatePage } from './create.tsx'

const SLUG = { slug: 'test' }
const CREATE_ROUTE = routeMount(automationsCreateRoute, SLUG)
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
    params: { slug: 'test', automationId: '7' },
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

test('Cancel returns to the list without calling the API', async () => {
  const { screen, navigate } = await renderWithRouter(<AutomationCreatePage />, CREATE_ROUTE)
  await screen.getByRole('button', { name: 'Cancel' }).click()

  expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ params: { slug: 'test' } }))
})
