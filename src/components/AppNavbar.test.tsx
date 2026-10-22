import { expect, test } from 'vitest'

import { Route as activityRoute } from '../routes/workspaces/$workspaceId/activity.tsx'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as brandKitRoute } from '../routes/workspaces/$workspaceId/brand-kit.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as customFieldsRoute } from '../routes/workspaces/$workspaceId/custom-fields/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as tagsRoute } from '../routes/workspaces/$workspaceId/tags/index.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$workspaceId/transactional-emails/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AppNavbar } from './AppNavbar.tsx'

const SECTIONS = [
  ['Overview', overviewRoute],
  ['Automations', automationsRoute],
  ['Analytics', analyticsRoute],
  ['Activity', activityRoute],
  ['Settings', settingsRoute],
] as const

// Under the People group, which is closed until one of its pages is viewed.
const PEOPLE_SECTIONS = [
  ['Contacts', contactsRoute],
  ['Segments', segmentsRoute],
  ['Tags', tagsRoute],
  ['Custom fields', customFieldsRoute],
] as const

// Under the Emails group, which is closed until one of its pages is viewed.
const EMAIL_SECTIONS = [
  ['Broadcasts', broadcastsRoute],
  ['Templates', templatesRoute],
  ['Transactional emails', transactionalEmailsRoute],
  ['Brand kit', brandKitRoute],
] as const

test.each(SECTIONS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { workspaceId: 'AcmeWs01' }).initialPath)
})

test.each(PEOPLE_SECTIONS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)
  await screen.getByText('People', { exact: true }).click()

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { workspaceId: 'AcmeWs01' }).initialPath)
})

test.each(EMAIL_SECTIONS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)
  await screen.getByText('Emails', { exact: true }).click()

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { workspaceId: 'AcmeWs01' }).initialPath)
})

test('only the section being viewed is current, the overview not on a page under it', async () => {
  const { screen } = await renderWithRouter(
    <AppNavbar workspaceId="AcmeWs01" />,
    routeMount(contactsRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Contacts', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await expect
    .element(screen.getByRole('link', { name: 'Overview', exact: true }))
    .not.toHaveAttribute('aria-current')
})

test('the people sections sit under one group, closed until one of them is viewed', async () => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)

  await expect.element(screen.getByText('People', { exact: true })).toBeVisible()
  await expect
    .element(screen.getByRole('link', { name: 'Contacts', exact: true }))
    .not.toBeInTheDocument()
})

test('the people group is open on one of its pages', async () => {
  const { screen } = await renderWithRouter(
    <AppNavbar workspaceId="AcmeWs01" />,
    routeMount(customFieldsRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect.element(screen.getByRole('link', { name: 'Contacts', exact: true })).toBeVisible()
})

test('the email sections sit under one group, closed until one of them is viewed', async () => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)

  await expect.element(screen.getByText('Emails', { exact: true })).toBeVisible()
  await expect
    .element(screen.getByRole('link', { name: 'Broadcasts', exact: true }))
    .not.toBeInTheDocument()
})

test('the email group is open on one of its pages', async () => {
  const { screen } = await renderWithRouter(
    <AppNavbar workspaceId="AcmeWs01" />,
    routeMount(transactionalEmailsRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect.element(screen.getByRole('link', { name: 'Broadcasts', exact: true })).toBeVisible()
})
