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

test('waits for the check, then says the domain is verified', async () => {
  let verified = false
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsVerify(() => {
      verified = true
      return HttpResponse.json(domain({ verified: true }))
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Verify' }).click()

  await expect.poll(() => verified).toBe(true)
  await expect.element(screen.getByText('Domain verified')).toBeInTheDocument()
})

test('says the domain is not verified yet when the check finds no record', async () => {
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsVerify(() => HttpResponse.json(domain())),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Verify' }).click()

  await expect.element(screen.getByText('Not verified yet')).toBeInTheDocument()
})

test('removes a domain', async () => {
  let removed = false
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsArchive(() => {
      removed = true
      return HttpResponse.json(domain({ archivedAt: '2026-02-01T00:00:00Z' }))
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()

  await expect.poll(() => removed).toBe(true)
  await expect.element(screen.getByText('Domain removed')).toBeInTheDocument()
})

test('removing a domain whose Tracking host serves asks first and says the tracker stops', async () => {
  let removed = false
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
      removed = true
      return HttpResponse.json({ ...serving, archivedAt: '2026-02-01T00:00:00Z' })
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()

  await expect
    .element(screen.getByText(/track\.mail\.acme\.com stops serving the tracker/))
    .toBeInTheDocument()
  expect(removed).toBe(false)

  await screen.getByRole('button', { name: 'Remove', exact: true }).click()
  await expect.poll(() => removed).toBe(true)
})

test('a domain that live work sends from is not removed and the work is named', async () => {
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
  await screen.getByRole('menuitem', { name: 'Remove' }).click()

  await expect.element(screen.getByText('Failed to remove domain')).toBeInTheDocument()
  await expect.element(screen.getByText(/Automation "Welcome series"/)).toBeInTheDocument()
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

test('clicking a domain row opens its DNS records in a modal', async () => {
  worker.use(list([domain()]))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('mail.acme.com').click()

  await expect.element(screen.getByText('om1._domainkey.mail.acme.com')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('textbox', { name: 'DMARC (recommended): Value' }))
    .toHaveValue('v=DMARC1; p=none')
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
  await expect
    .element(screen.getByRole('textbox', { name: 'Tracking domain (optional): Value' }))
    .toHaveValue('edge.example.com')
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

test('shows the records of a domain right after it is added', async () => {
  let created = false
  worker.use(
    handleSiteSendingDomainsList(() => {
      const items = created ? [domain()] : []
      return HttpResponse.json({
        items,
        page: 1,
        pageSize: 20,
        totalItems: items.length,
        totalPages: 1,
      })
    }),
    handleSiteSendingDomainsCreate(() => {
      created = true
      return HttpResponse.json(domain(), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByRole('button', { name: 'Add domain' }).click()
  await screen
    .getByRole('dialog')
    .getByLabelText(/^Domain/)
    .fill('mail.acme.com')
  await screen.getByRole('dialog').getByRole('button', { name: 'Add domain' }).click()

  await expect.element(screen.getByText('om1._domainkey.mail.acme.com')).toBeInTheDocument()
})

test('verifies a domain from its records modal and offers the agent prompt', async () => {
  let verified = false
  worker.use(
    list([domain()]),
    handleSiteSendingDomainsVerify(() => {
      verified = true
      return HttpResponse.json(domain())
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('mail.acme.com').click()
  await screen.getByRole('tab', { name: 'Prompt for agent' }).click()
  await expect
    .element(screen.getByRole('textbox', { name: 'Copy prompt for agent' }))
    .toHaveValue(expect.stringContaining('Set up the Sending domain mail.acme.com'))
  await expect
    .element(screen.getByRole('button', { name: 'Copy prompt for agent' }))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Verify now' }).click()

  await expect.poll(() => verified).toBe(true)
})

// The list answers by the view asked: the kept domains, or the archived ones.
const byView = (kept: SiteSendingDomainResource[], archived: SiteSendingDomainResource[]) =>
  handleSiteSendingDomainsList(({ request }) => {
    const items = new URL(request.url).searchParams.get('archived') === 'true' ? archived : kept
    return HttpResponse.json({
      items,
      page: 1,
      pageSize: 20,
      totalItems: items.length,
      totalPages: 1,
    })
  })

test('switches to the archived domains and restores one', async () => {
  const restored: string[] = []
  worker.use(
    byView(
      [domain()],
      [domain({ id: '9', domain: 'old.acme.com', archivedAt: '2026-02-01T00:00:00Z' })],
    ),
    handleSiteSendingDomainsUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(domain({ id: '9', domain: 'old.acme.com' }))
    }),
  )
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await expect.element(screen.getByText('mail.acme.com')).toBeInTheDocument()
  await screen.getByText('Archived', { exact: true }).click()
  await expect.element(screen.getByText('old.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('mail.acme.com')).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).click()
  await expect.element(screen.getByRole('menuitem', { name: 'Remove' })).not.toBeInTheDocument()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['9'])
})

test('an empty archive of domains says so', async () => {
  worker.use(byView([domain()], []))
  const { screen } = await renderWithRouter(
    <SendingDomainsSection workspaceId={WORKSPACE_ID} canEdit />,
  )

  await screen.getByText('Archived', { exact: true }).click()

  await expect.element(screen.getByText('No archived domains')).toBeInTheDocument()
})
