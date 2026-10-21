import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteDomainJoinGet,
  handleSiteDomainJoinSet,
  handleSiteJoinDomainsAdd,
  handleSiteJoinDomainsList,
  handleSiteJoinDomainsRemove,
  handleSiteJoinDomainsVerify,
} from '../../generated/site/msw.gen.ts'
import type { SiteJoinDomainResource } from '../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { JoinDomainsSection } from './JoinDomainsSection.tsx'

const WORKSPACE_ID = 'TestWs01'

// Every test starts with the switch off; the ones about it override this.
beforeEach(() => {
  worker.use(handleSiteDomainJoinGet({ body: { enabled: false } }))
})

function domain(over: Partial<SiteJoinDomainResource> = {}): SiteJoinDomainResource {
  return {
    id: '1',
    domain: 'acme.com',
    verified: false,
    record: { type: 'TXT', host: '_sphericon-join.acme.com', value: 'sphericon-join=abc' },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }
}

test('shows a pending domain with the record to publish and a verified one without', async () => {
  worker.use(
    handleSiteJoinDomainsList({
      body: [domain(), domain({ id: '2', domain: 'eu.acme.com', verified: true })],
    }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('_sphericon-join.acme.com')).toBeInTheDocument()
  await expect.element(screen.getByText('sphericon-join=abc')).toBeInTheDocument()
  await expect.element(screen.getByText('Pending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Verified', { exact: true })).toBeInTheDocument()
})

test('adds a domain', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteJoinDomainsList({ body: [] }),
    handleSiteJoinDomainsAdd(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(domain(), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Add domain' }).click()
  await screen.getByLabelText(/^Domain/).fill(' acme.com ')
  await screen.getByRole('dialog').getByRole('button', { name: 'Add domain' }).click()

  await expect.poll(() => bodies).toEqual([{ domain: 'acme.com' }])
})

test('verifies a pending domain', async () => {
  let verified = false
  worker.use(
    handleSiteJoinDomainsList({ body: [domain()] }),
    handleSiteJoinDomainsVerify(() => {
      verified = true
      return HttpResponse.json(domain({ verified: true }))
    }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => verified).toBe(true)
})

test('removes a domain after confirmation', async () => {
  let removed = false
  worker.use(
    handleSiteJoinDomainsList({ body: [domain()] }),
    handleSiteJoinDomainsRemove(() => {
      removed = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Remove' }).click()

  await expect.poll(() => removed).toBe(true)
})

test('shows the domain join switch as stored', async () => {
  worker.use(
    handleSiteJoinDomainsList({ body: [] }),
    handleSiteDomainJoinGet({ body: { enabled: true } }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByRole('switch')).toBeChecked()
})

test('switches domain join on', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteJoinDomainsList({ body: [] }),
    handleSiteDomainJoinGet({ body: { enabled: false } }),
    handleSiteDomainJoinSet(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ enabled: true })
    }),
  )
  const { screen } = await renderWithRouter(<JoinDomainsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('switch').click()

  await expect.poll(() => bodies).toEqual([{ enabled: true }])
})
