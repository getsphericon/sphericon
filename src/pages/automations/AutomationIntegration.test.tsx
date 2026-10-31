import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })
const NOW = '2026-01-01T00:00:00Z'
const DRAFT: SiteAutomationVersion = {
  number: 1,
  triggerEvent: 'email.clicked',
  exitEvents: [],
  goalEvent: null,
  steps: [{ type: 'wait', seconds: 60 }],
}
const STORED: SiteAutomationResource = {
  id: '5',
  name: 'Welcome flow',
  status: 'inactive',
  live: null,
  draft: DRAFT,
  createdAt: NOW,
  updatedAt: NOW,
}

const integration = (
  id: string,
  name: string,
  extra: Partial<SiteIntegrationResource> = {},
): SiteIntegrationResource => ({
  id,
  name,
  channel: 'email',
  provider: 'smtp',
  enabled: true,
  isDefault: false,
  maxPerSecond: null,
  maxPerDay: null,
  providerLimit: {
    perSecond: { limit: null, source: null },
    perDay: { limit: null, source: null },
    sentLast24h: 0,
    warnings: ['unlimited'],
  },
  config: { kind: 'smtp', host: 'smtp.test', port: 587, from: 'a@acme.com' },
  preset: null,
  params: {},
  secretsRetiring: [],
  hookUrl: 'https://app.test/hooks/omik_one',
  createdAt: NOW,
  updatedAt: NOW,
  ...extra,
})

const INTEGRATIONS = [
  integration('1', 'Primary SMTP', { isDefault: true }),
  integration('21', 'Mailgun relay'),
]

const enrollments = handleSiteEnrollmentsList({
  body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
})

test('the Integration of the email steps is chosen in the bar and saved with the automation', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(STORED)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...STORED, draft: { ...DRAFT, integrationId: '21' } })
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    enrollments,
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await screen.getByRole('combobox', { name: 'Integration' }).click()
  await screen.getByRole('option', { name: 'Mailgun relay' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([expect.objectContaining({ integrationId: '21' })])
})

test('choosing the workspace default saves null and the picker lists email Integrations only', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet(() =>
      HttpResponse.json({ ...STORED, draft: { ...DRAFT, integrationId: '21' } }),
    ),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
    handleSiteIntegrationsList({
      body: [...INTEGRATIONS, integration('22', 'Text relay', { channel: 'sms' })],
    }),
    enrollments,
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  const picker = screen.getByRole('combobox', { name: 'Integration' })
  await expect.element(picker).toHaveValue('Mailgun relay')

  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Text relay' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Workspace default' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([expect.objectContaining({ integrationId: null })])
})
