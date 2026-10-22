import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAnalyticsOverview,
  handleSiteSendingDomainsList,
  handleSiteSendingDomainsVerify,
  handleSiteSetupChecklistGet,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteSendingDomainResource,
  SiteSetupStepStatus,
} from '../../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { OverviewPage } from './overview.tsx'

const mount = routeMount(overviewRoute, { workspaceId: 'TestWs01' })

const checklist = (sendingDomain: SiteSetupStepStatus) =>
  ({
    steps: { integration: 'done', sendingDomain, contact: 'todo', sentEmail: 'todo' },
    complete: false,
  }) as const

const domain: SiteSendingDomainResource = {
  id: '1',
  domain: 'mail.acme.com',
  dkimSelector: 'om1',
  verified: false,
  dkimRecord: { type: 'TXT', host: 'om1._domainkey.mail.acme.com', value: 'v=DKIM1; p=KEY' },
  spfRecord: { type: 'TXT', host: 'mail.acme.com', value: 'v=spf1 include:x -all' },
  dmarcRecord: { type: 'TXT', host: '_dmarc.mail.acme.com', value: 'v=DMARC1; p=none' },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const domains = (items: SiteSendingDomainResource[]) =>
  handleSiteSendingDomainsList({
    body: { items, page: 1, pageSize: 20, totalItems: items.length, totalPages: 1 },
  })

test('a pending Sending domain step shows the DKIM record and keeps steps 3 and 4 linked', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: checklist('pending') }),
    domains([domain]),
    handleSiteWorkspacesList({ body: [] }),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('om1._domainkey.mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('v=DKIM1; p=KEY')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Copy DKIM value' })).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Open Contacts' })).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Create a Broadcast' })).toBeInTheDocument()
})

test('check now verifies the domain and the step turns done without a reload', async () => {
  let verified = false
  worker.use(
    handleSiteSetupChecklistGet(() => HttpResponse.json(checklist(verified ? 'done' : 'pending'))),
    domains([domain]),
    handleSiteSendingDomainsVerify(() => {
      verified = true
      return new HttpResponse(null, { status: 204 })
    }),
    handleSiteWorkspacesList({ body: [] }),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await screen.getByRole('button', { name: 'Check now' }).click()

  await expect.element(screen.getByText('Waiting for DNS')).not.toBeInTheDocument()
  await expect.element(screen.getByText('v=DKIM1; p=KEY')).not.toBeInTheDocument()
})

test('a failing check keeps the step pending and says so', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: checklist('pending') }),
    domains([domain]),
    handleSiteSendingDomainsVerify(() => problem(500, { detail: 'dns down' })),
    handleSiteWorkspacesList({ body: [] }),
    handleSiteAnalyticsOverview(() => problem(500)),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await screen.getByRole('button', { name: 'Check now' }).click()

  await expect.element(screen.getByText('dns down')).toBeInTheDocument()
  await expect.element(screen.getByText('Waiting for DNS')).toBeInTheDocument()
})
