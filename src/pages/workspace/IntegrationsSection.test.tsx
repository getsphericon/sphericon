import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAuditList,
  handleSiteIntegrationsRegenerateKey,
  handleSitePresetsList,
  handleSiteIntegrationsCreate,
  handleSiteIntegrationsArchive,
  handleSiteIntegrationsList,
  handleSiteIntegrationsUnarchive,
  handleSiteIntegrationsUpdate,
  handleSiteMembershipsList,
  handleSiteUserGetMe,
} from '../../generated/site/msw.gen.ts'
import type { SiteIntegrationResource, SiteMembershipRole } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { IntegrationsSection } from './IntegrationsSection.tsx'

const WORKSPACE_ID = 'TestWs01'

const smtp: SiteIntegrationResource = {
  id: '1',
  name: 'Primary SMTP',
  channel: 'email',
  provider: 'smtp',
  enabled: true,
  isDefault: true,
  maxPerSecond: null,
  maxPerDay: null,
  sendLimit: {
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
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

// The signed-in User holds `role` in the Workspace (writes are owner/admin-only).
const signedInAs = (role: SiteMembershipRole) => [
  handleSiteUserGetMe({
    body: {
      id: '10',
      name: 'Ann',
      email: 'ann@acme.com',
      emailVerified: true,
      createdAt: '2026-01-01T00:00:00Z',
    },
  }),
  handleSiteMembershipsList({
    body: [
      {
        id: '1',
        userId: '10',
        email: 'ann@acme.com',
        name: 'Ann',
        role,
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
  }),
]

const list = (items: SiteIntegrationResource[]) => handleSiteIntegrationsList({ body: items })

const create = (bodies: unknown[]) =>
  handleSiteIntegrationsCreate(async ({ request }) => {
    bodies.push(await request.json())
    return HttpResponse.json(smtp, { status: 201 })
  })

// Without a license the change-history link probes the Audit log and hides itself.
beforeEach(() => {
  worker.use(
    handleSiteAuditList(() => problem(402)),
    ...signedInAs('owner'),
  )
})

test('lists connected providers', async () => {
  worker.use(list([smtp]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
})

test('connects an SMTP provider', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Name/).fill(' Mailer ')
  await dialog.getByLabelText(/^SMTP host/).fill('smtp.acme.com')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByLabelText('Set as default').click()
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Mailer',
        isDefault: true,
        maxPerSecond: null,
        maxPerDay: null,
        config: {
          kind: 'smtp',
          host: 'smtp.acme.com',
          port: 587,
          username: null,
          password: null,
          from: 'hi@acme.com',
          fromName: null,
        },
      },
    ])
})

test('connects an Amazon SES provider', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('combobox', { name: 'Provider' }).click()
  await screen.getByRole('option', { name: 'Amazon SES' }).click()
  await dialog.getByLabelText(/^Name/).fill('SES')
  await dialog.getByLabelText(/^AWS region/).fill('eu-west-1')
  await dialog.getByLabelText(/^Access key ID/).fill('AKIA')
  await dialog.getByLabelText(/^Secret access key/).fill('shh')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByLabelText(/^From name/).fill('Acme')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'SES',
        isDefault: false,
        maxPerSecond: null,
        maxPerDay: null,
        config: {
          kind: 'ses',
          region: 'eu-west-1',
          accessKeyId: 'AKIA',
          secretAccessKey: 'shh',
          endpoint: null,
          from: 'hi@acme.com',
          fromName: 'Acme',
        },
      },
    ])
})

test('archives a provider', async () => {
  let archived = false
  worker.use(
    list([smtp]),
    handleSiteIntegrationsArchive(() => {
      archived = true
      return HttpResponse.json({ ...smtp, isDefault: false, archivedAt: '2026-02-01T00:00:00Z' })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toBe(true)
})

test('restores an archived provider from the archived view', async () => {
  let restored = false
  worker.use(
    handleSiteIntegrationsList(({ request }) =>
      HttpResponse.json(
        new URL(request.url).searchParams.get('archived') === 'true'
          ? [{ ...smtp, isDefault: false, archivedAt: '2026-02-01T00:00:00Z' }]
          : [],
      ),
    ),
    handleSiteIntegrationsUnarchive(() => {
      restored = true
      return HttpResponse.json({ ...smtp, isDefault: false })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByLabelText('Show archived providers').click()
  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toBe(true)
})

test('a provider that live work uses is not archived and the work is named', async () => {
  worker.use(
    list([smtp]),
    handleSiteIntegrationsArchive(() =>
      problem(409, { usedBy: [{ kind: 'broadcast', id: '7', name: 'Summer sale teaser' }] }),
    ),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.element(screen.getByText('Failed to archive provider')).toBeInTheDocument()
  await expect.element(screen.getByText(/Broadcast "Summer sale teaser"/)).toBeInTheDocument()
})
test('shows an error alert when providers fail to load', async () => {
  worker.use(handleSiteIntegrationsList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Failed to load providers').first()).toBeInTheDocument()
})

test('reports a create failure', async () => {
  worker.use(
    list([]),
    handleSiteIntegrationsCreate(() => problem(422, { detail: 'bad host' })),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Name/).fill('X')
  await dialog.getByLabelText(/^SMTP host/).fill('h')
  await dialog.getByLabelText(/^From address/).fill('a@b.co')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect.element(screen.getByText('Failed to save provider')).toBeInTheDocument()
})

const limited: SiteIntegrationResource = {
  ...smtp,
  maxPerSecond: 14,
  maxPerDay: 50000,
  sendLimit: {
    perSecond: { limit: 14, source: 'manual' },
    perDay: { limit: 50000, source: 'manual' },
    sentLast24h: 1200,
    warnings: [],
  },
}

const update = (bodies: unknown[]) =>
  handleSiteIntegrationsUpdate(async ({ request }) => {
    bodies.push(await request.json())
    return HttpResponse.json(limited)
  })

test('warns when an Integration has no send limit', async () => {
  worker.use(list([smtp]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Primary SMTP has no send limit')).toBeInTheDocument()
  await expect.element(screen.getByText('Per second: Unlimited')).toBeInTheDocument()
})

test('warns when the provider send quota could not be read', async () => {
  const ses: SiteIntegrationResource = {
    ...limited,
    name: 'Amazon SES',
    sendLimit: { ...limited.sendLimit, warnings: ['providerQuotaUnavailable'] },
  }
  worker.use(list([ses]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect
    .element(screen.getByText("Amazon SES: the provider's send quota could not be read"))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText('Without the provider quota', { exact: false }))
    .toBeInTheDocument()
})

test('shows no quota warning when the provider quota is known', async () => {
  worker.use(list([limited]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Per second: 14')).toBeInTheDocument()
  await expect
    .element(screen.getByText('send quota could not be read', { exact: false }))
    .not.toBeInTheDocument()
})

test('shows the effective limit, its source and the 24-hour usage', async () => {
  worker.use(list([limited]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Per second: 14')).toBeInTheDocument()
  await expect.element(screen.getByText('Per 24 h: 50 000')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Sent in the last 24 h: 1 200 / 50 000'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Manual').first()).toBeInTheDocument()
  await expect.element(screen.getByText('has no send limit')).not.toBeInTheDocument()
})

test('connects a provider with send limits', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Name/).fill('Mailer')
  await dialog.getByLabelText(/^SMTP host/).fill('smtp.acme.com')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByLabelText('Max per second').fill('5')
  await dialog.getByLabelText('Max per 24 hours').fill('1000')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ name: 'Mailer', maxPerSecond: 5, maxPerDay: 1000 }])
})

test('edits only the limits of an existing provider, and a blank clears one', async () => {
  const bodies: unknown[] = []
  worker.use(list([limited]), update(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Limits' }).click()
  const dialog = screen.getByRole('dialog')
  await expect.element(dialog.getByLabelText('Max per second')).toHaveValue('14')
  await dialog.getByLabelText('Max per second').fill('20')
  await dialog.getByLabelText('Max per 24 hours').clear()
  await dialog.getByRole('button', { name: 'Save limits' }).click()

  await expect.poll(() => bodies).toEqual([{ maxPerSecond: 20, maxPerDay: null }])
})

test('a member can read the providers but not change them', async () => {
  worker.use(list([smtp]), ...signedInAs('member'))
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Connect provider' }))
    .not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Archive' })).not.toBeInTheDocument()
})

const generic = {
  name: 'generic',
  description: 'A body already in the Delivery failure shape, signed with Standard Webhooks.',
  paramsSchema: {
    type: 'object',
    properties: { secret: { type: 'string', writeOnly: true } },
  },
}

const withPreset: SiteIntegrationResource = {
  ...smtp,
  preset: 'generic',
  params: { secret: '********' },
  secretsRetiring: ['secret'],
}

test('shows the hook URL and chooses a Preset with its params', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([smtp]),
    handleSitePresetsList({ body: [generic] }),
    handleSiteIntegrationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(withPreset)
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Inbound' }).click()
  await expect.element(screen.getByText('https://app.test/hooks/omik_one')).toBeInTheDocument()
  await screen.getByRole('combobox', { name: 'Preset' }).click()
  await screen.getByRole('option', { name: 'generic' }).click()
  await screen.getByRole('textbox', { name: /^Params/ }).fill('{"secret": "whsec_abc"}')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ preset: 'generic', params: { secret: 'whsec_abc' } }])
})

test('retires the old secrets of a rotating Preset', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([withPreset]),
    handleSitePresetsList({ body: [generic] }),
    handleSiteIntegrationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...withPreset, secretsRetiring: [] })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Inbound' }).click()
  await screen.getByRole('button', { name: 'Retire old secrets' }).click()

  await expect.poll(() => bodies).toEqual([{ retireSecrets: true }])
})

test('regenerates the hook URL after a confirmation', async () => {
  const calls: string[] = []
  worker.use(
    list([smtp]),
    handleSitePresetsList({ body: [generic] }),
    handleSiteIntegrationsRegenerateKey(({ params }) => {
      calls.push(params.id)
      return HttpResponse.json({ ...smtp, hookUrl: 'https://app.test/hooks/omik_two' })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Inbound' }).click()
  await screen.getByRole('button', { name: 'Regenerate URL' }).click()
  await screen
    .getByRole('dialog', { name: 'Regenerate the hook URL?' })
    .getByRole('button', { name: 'Regenerate URL' })
    .click()

  await expect.poll(() => calls).toEqual(['1'])
})
