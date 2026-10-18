import { expect, test } from 'vitest'

import { Route as activityRoute } from '../routes/workspaces/$slug/activity.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$slug/automations/index.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$slug/contacts/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$slug/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$slug/settings.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$slug/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$slug/transactional-emails/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { AppNavbar } from './AppNavbar.tsx'

const SECTIONS = [
  ['Overview', overviewRoute],
  ['Contacts', contactsRoute],
  ['Segments', segmentsRoute],
  ['Broadcasts', broadcastsRoute],
  ['Templates', templatesRoute],
  ['Automations', automationsRoute],
  ['Transactional emails', transactionalEmailsRoute],
  ['Activity', activityRoute],
  ['Settings', settingsRoute],
] as const

test.each(SECTIONS)('%s navigates to its workspace-scoped route', async (label, target) => {
  const { screen, navigate } = await renderWithRouter(<AppNavbar slug="acme" />)

  await screen.getByText(label, { exact: true }).click()

  expect(navigate).toHaveBeenCalledWith({ to: target.to, params: { slug: 'acme' } })
})
