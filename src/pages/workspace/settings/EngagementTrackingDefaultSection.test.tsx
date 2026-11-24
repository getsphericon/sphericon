import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetEngagementTrackingDefault,
} from '../../../generated/site/msw.gen.ts'
import type {
  SiteSetEngagementTrackingDefaultInput,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { workspace as workspacePayload } from '../../../test/payloads.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { worker } from '../../../test/worker.ts'
import { EngagementTrackingDefaultSection } from './EngagementTrackingDefaultSection.tsx'

const workspace = (over: Partial<SiteWorkspaceResource> = {}): SiteWorkspaceResource => ({
  ...workspacePayload(),
  id: 'AcmeWs01',
  revision: 3,
  permissions: ['workspace:update'],
  role: 'admin',
  ...over,
})

test('turning the tracking default to dont_track says what it withdraws, then sends it', async () => {
  const sent: SiteSetEngagementTrackingDefaultInput[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteWorkspacesSetEngagementTrackingDefault(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(workspace({ engagementTrackingDefault: 'dont_track' }))
    }),
  )

  const { screen } = await renderWithRouter(
    <EngagementTrackingDefaultSection workspace={workspace()} />,
  )

  await expect
    .element(screen.getByText('Track: every recipient is tracked unless they refused.'))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Track only those who agree' }).click()
  await expect
    .element(screen.getByText(/messages already sent to recipients who did not grant/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/the opens and clicks recorded of them are kept/))
    .toBeInTheDocument()
  await screen.getByRole('dialog').getByRole('button', { name: 'Change' }).click()

  await expect.poll(() => sent).toEqual([{ revision: 3, engagementTrackingDefault: 'dont_track' }])
})

test('a member sees the tracking default and cannot change it', async () => {
  const member = workspace({
    permissions: [],
    role: 'member',
    engagementTrackingDefault: 'dont_track',
  })
  worker.use(handleSiteWorkspacesList({ body: [member] }))

  const { screen } = await renderWithRouter(<EngagementTrackingDefaultSection workspace={member} />)

  await expect
    .element(
      screen.getByText("Don't track: only recipients who granted their consent are tracked."),
    )
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: /Track/ })).not.toBeInTheDocument()
})
