import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesGetSendingRules,
  handleSiteWorkspacesList,
  handleSiteWorkspacesReplaceSendingRules,
} from '../../../generated/site/msw.gen.ts'
import type {
  Permission,
  SiteSendingRulesResource,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { workspace as workspacePayload } from '../../../test/payloads.ts'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { worker } from '../../../test/worker.ts'
import { SendingRulesSection } from './SendingRulesSection.tsx'

const workspace = (permissions: Permission[]): SiteWorkspaceResource => ({
  ...workspacePayload(),
  postalAddress: '',
  permissions,
})

const defaults: SiteSendingRulesResource = {
  frequencyCaps: [{ channel: 'sms', country: null, maxMessages: 3, periodMinutes: 1440 }],
  sendingWindows: [
    { channel: 'sms', country: null, hours: [{ day: 'monday', opensAt: 480, closesAt: 1200 }] },
  ],
  revision: 3,
}

test('workspace:update changes a cap and saves the rules on the revision it read', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace(['workspace:update'])] }),
    handleSiteWorkspacesGetSendingRules({ body: defaults }),
    handleSiteWorkspacesReplaceSendingRules(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(defaults)
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingRulesSection workspace={workspace(['workspace:update'])} />,
  )

  await expect
    .element(screen.getByText(/SMS: at most 3 marketing messages to one recipient per 1 day/))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/no table of laws/)).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Change' }).click()
  await screen.getByRole('textbox', { name: 'Maximum' }).fill('2')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        frequencyCaps: [{ channel: 'sms', country: null, maxMessages: 2, periodMinutes: 1440 }],
        sendingWindows: defaults.sendingWindows,
        revision: 3,
      },
    ])
})

test('without workspace:update the rules show and offer no change', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace([])] }),
    handleSiteWorkspacesGetSendingRules({ body: defaults }),
  )
  const { screen } = await renderWithRouter(<SendingRulesSection workspace={workspace([])} />)

  await expect.element(screen.getByText(/SMS: Monday 08:00–20:00/)).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Change' })).not.toBeInTheDocument()
})

// The server names a window's day by its place among the hours sent: its refusal shows under the
// day's own input, and no toast repeats it.
test('a refusal of a window day shows under that day', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace(['workspace:update'])] }),
    handleSiteWorkspacesGetSendingRules({ body: defaults }),
    handleSiteWorkspacesReplaceSendingRules(() =>
      problem(422, {
        title: 'Unprocessable',
        errors: { 'sendingWindows.0.hours.0.closesAt': ['Close after it opens.'] },
      }),
    ),
  )
  const { screen } = await renderWithRouter(
    <SendingRulesSection workspace={workspace(['workspace:update'])} />,
  )

  await screen.getByRole('button', { name: 'Change' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Close after it opens.')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('textbox', { name: 'Monday closes at' }))
    .toHaveAttribute('aria-invalid', 'true')
  await expect.element(screen.getByText('Could not save the sending rules')).not.toBeInTheDocument()
})

// The rules changed since the form read them (ADR 0119): the editor says so and offers to load the
// saved version or save over it.
test('a save over a change made since shows the changed notice', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace(['workspace:update'])] }),
    handleSiteWorkspacesGetSendingRules({ body: defaults }),
    handleSiteWorkspacesReplaceSendingRules(() =>
      problem(409, { title: 'Changed', code: 'revision_changed' }),
    ),
  )
  const { screen } = await renderWithRouter(
    <SendingRulesSection workspace={workspace(['workspace:update'])} />,
  )

  await screen.getByRole('button', { name: 'Change' }).click()
  await screen.getByRole('textbox', { name: 'Maximum' }).fill('2')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .element(screen.getByRole('button', { name: 'Load the saved version' }))
    .toBeInTheDocument()
})
