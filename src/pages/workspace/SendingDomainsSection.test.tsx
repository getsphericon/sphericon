import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSendingDomainsCreate,
  handleSiteSendingDomainsArchive,
  handleSiteSendingDomainsList,
  handleSiteSendingDomainsUnarchive,
  handleSiteSendingDomainsVerify,
} from '../../generated/site/msw.gen.ts'
import type { SiteSendingDomainResource } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { SendingDomainsSection } from './SendingDomainsSection.tsx'

const WORKSPACE_ID = 'TestWs01'

function domain(over: Partial<SiteSendingDomainResource> = {}): SiteSendingDomainResource {
  return {
    id: '1',
    domain: 'mail.acme.com',
    dkimSelector: 'om1',
    verified: false,
    dkimRecord: { type: 'TXT', host: 'om1._domainkey.mail.acme.com', value: 'v=DKIM1; p=KEY' },
    spfRecord: { type: 'TXT', host: 'mail.acme.com', value: 'v=spf1 include:x -all' },
    dmarcRecord: { type: 'TXT', host: '_dmarc.mail.acme.com', value: 'v=DMARC1; p=none' },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }
}

const list = (items: SiteSendingDomainResource[]) =>
  handleSiteSendingDomainsList({
    body: { items, page: 1, pageSize: 20, totalItems: items.length, totalPages: 1 },
  })

test('shows pending and verified domains', async () => {
  worker.use(list([domain(), domain({ id: '2', domain: 'ok.acme.com', verified: true })]))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await expect.element(screen.getByText('mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('Pending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Verified', { exact: true })).toBeInTheDocument()
})

test('offers a member the domains to read, without adding or managing them', async () => {
  worker.use(list([domain()]))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit={false} />,
  )

  await expect.element(screen.getByText('mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Add domain' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'More actions' })).not.toBeInTheDocument()
})

test('shows a domain whose verification was lost apart from one never verified', async () => {
  worker.use(
    list([
      domain(),
      domain({
        id: '2',
        domain: 'lost.acme.com',
        verifiedAt: '2026-01-02T00:00:00Z',
        verificationLostAt: '2026-02-01T00:00:00Z',
      }),
    ]),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await expect.element(screen.getByText('Pending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Verification lost', { exact: true })).toBeInTheDocument()
})

test('adds a domain', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([]),
    handleSiteSendingDomainsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(domain(), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'Add domain' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Domain/).fill(' mail.acme.com ')
  await dialog.getByRole('button', { name: 'Add domain' }).click()

  await expect.poll(() => bodies).toEqual([{ domain: 'mail.acme.com' }])
})

test('starts verification', async () => {
  let verified = false
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsVerify(() => {
      verified = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Verify' }).click()

  await expect.poll(() => verified).toBe(true)
  await expect.element(screen.getByText('Verification started')).toBeInTheDocument()
})

test('archives a domain', async () => {
  let archived = false
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsArchive(() => {
      archived = true
      return HttpResponse.json(domain({ archivedAt: '2026-02-01T00:00:00Z' }))
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toBe(true)
  await expect.element(screen.getByText('Domain archived')).toBeInTheDocument()
})

test('archiving a domain whose Tracking host serves asks first and says the tracker stops', async () => {
  let archived = false
  const serving = domain({
    trackingRecord: {
      type: 'CNAME',
      host: 'track.mail.acme.com',
      target: 'edge.sphericon.test',
      state: 'serving',
    },
  })
  worker.use(
    list([serving]),
    handleSiteSendingDomainsArchive(() => {
      archived = true
      return HttpResponse.json({ ...serving, archivedAt: '2026-02-01T00:00:00Z' })
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect
    .element(screen.getByText(/track\.mail\.acme\.com stops serving the tracker/))
    .toBeInTheDocument()
  expect(archived).toBe(false)

  await screen.getByRole('button', { name: 'Archive', exact: true }).click()
  await expect.poll(() => archived).toBe(true)
})

test('a domain that live work sends from is not archived and the work is named', async () => {
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsArchive(() =>
      problem(409, { usedBy: [{ kind: 'automation', id: '3', name: 'Welcome series' }] }),
    ),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.element(screen.getByText('Failed to archive domain')).toBeInTheDocument()
  await expect.element(screen.getByText(/Automation "Welcome series"/)).toBeInTheDocument()
})

test('lists archived domains with the switch and restores one', async () => {
  const queries: (string | null)[] = []
  let restored = false
  worker.use(
    handleSiteSendingDomainsList(({ request }) => {
      queries.push(new URL(request.url).searchParams.get('archived'))
      const items = [domain({ archivedAt: '2026-02-01T00:00:00Z' })]
      return HttpResponse.json({ items, page: 1, pageSize: 20, totalItems: 1, totalPages: 1 })
    }),
    handleSiteSendingDomainsUnarchive(() => {
      restored = true
      return HttpResponse.json(domain())
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByLabelText('Show archived').click()
  await expect.poll(() => queries).toContain('true')

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toBe(true)
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteSendingDomainsList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await expect
    .element(screen.getByText('Failed to load sending domains').first())
    .toBeInTheDocument()
})

test('expanding a domain row shows its DNS records', async () => {
  worker.use(list([domain()]))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('mail.acme.com').click()

  await expect.element(screen.getByText('om1._domainkey.mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('v=DMARC1; p=none')).toBeInTheDocument()
})

test('lists the tracking CNAME with its state when the instance has a target', async () => {
  worker.use(
    list([
      domain({
        trackingRecord: {
          type: 'CNAME',
          host: 'track.mail.acme.com',
          target: 'edge.example.com',
          state: 'dns_verified',
        },
      }),
    ]),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('mail.acme.com').click()

  await expect.element(screen.getByText('track.mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('edge.example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('DNS verified')).toBeInTheDocument()
})

test('lists no tracking record when the instance has no target', async () => {
  worker.use(list([domain()]))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('mail.acme.com').click()

  await expect.element(screen.getByText('_dmarc.mail.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('Tracking domain (optional)')).not.toBeInTheDocument()
})
