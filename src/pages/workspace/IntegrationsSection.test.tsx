import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteAuditList,
  handleSiteIntegrationsRegenerateKey,
  handleSitePresetsList,
  handleSiteIntegrationsCreate,
  handleSiteIntegrationsArchive,
  handleSiteIntegrationsList,
  handleSiteIntegrationsUpdate,
} from '../../generated/site/msw.gen.ts'
import type {
  Permission,
  SiteIntegrationResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { IntegrationsSection } from './IntegrationsSection.tsx'

const smtp: SiteIntegrationResource = {
  id: '1',
  name: 'Primary SMTP',
  channel: 'email',
  provider: 'smtp',
  enabled: true,
  isDefault: true,
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
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

// The Workspace as the signed-in User sees it: writing Integrations needs integrations:manage.
const workspaceWith = (permissions: Permission[]): SiteWorkspaceResource => ({
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions,
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
})
const manager = workspaceWith(['integrations:manage'])

const list = (items: SiteIntegrationResource[]) => handleSiteIntegrationsList({ body: items })

const create = (bodies: unknown[]) =>
  handleSiteIntegrationsCreate(async ({ request }) => {
    bodies.push(await request.json())
    return HttpResponse.json(smtp, { status: 201 })
  })

// Without a license the change-history link probes the Audit log and hides itself.
beforeEach(() => {
  worker.use(handleSiteAuditList(() => problem(402)))
})

test('lists connected providers', async () => {
  worker.use(list([smtp]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
})

test('connects an SMTP provider and names it after its host', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'SMTP' }).click()
  await dialog.getByLabelText(/^SMTP host/).fill('smtp.acme.com')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'smtp.acme.com',
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

test('a second provider is the default only when asked, and may be named', async () => {
  const bodies: unknown[] = []
  worker.use(list([smtp]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'SMTP' }).click()
  await dialog.getByLabelText(/^SMTP host/).fill('smtp.acme.com')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByRole('button', { name: 'Advanced settings' }).click()
  await dialog.getByLabelText(/^Name/).fill(' Mailer ')
  await dialog.getByLabelText('Set as default').click()
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect.poll(() => bodies).toMatchObject([{ name: 'Mailer', isDefault: true }])
})

test('connects an Amazon SES provider', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Amazon SES' }).click()
  await dialog.getByLabelText(/^AWS region/).fill('eu-west-1')
  await dialog.getByLabelText(/^Access key ID/).fill('AKIA')
  await dialog.getByLabelText(/^Secret access key/).fill('shh')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByRole('button', { name: 'Advanced settings' }).click()
  await dialog.getByLabelText(/^From name/).fill('Acme')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Amazon SES (eu-west-1)',
        isDefault: true,
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

test('connects an SES-compatible service at its own endpoint, with its Provider limit', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'SES-compatible service' }).click()
  await dialog.getByLabelText(/^Custom endpoint/).fill('https://postbox.cloud.yandex.net')
  await dialog.getByLabelText(/^AWS region/).fill('ru-central1')
  await dialog.getByLabelText(/^Access key ID/).fill('YC')
  await dialog.getByLabelText(/^Secret access key/).fill('shh')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByLabelText(/^Max per second/).fill('1')
  await dialog.getByLabelText(/^Max per 24 hours/).fill('200')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([
      {
        name: 'postbox.cloud.yandex.net',
        maxPerSecond: 1,
        maxPerDay: 200,
        config: {
          kind: 'ses',
          endpoint: 'https://postbox.cloud.yandex.net',
          region: 'ru-central1',
        },
      },
    ])
})

test('Amazon SES asks for no endpoint', async () => {
  worker.use(list([]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Amazon SES' }).click()

  await expect.element(dialog.getByLabelText(/^Custom endpoint/)).not.toBeInTheDocument()
})

test('connects the Test inbox with no credentials form', async () => {
  const bodies: unknown[] = []
  worker.use(list([smtp]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Test inbox' }).click()

  await expect
    .element(dialog.getByText(/keeps every message it is given and delivers none/))
    .toBeInTheDocument()
  await expect.element(dialog.getByLabelText(/^SMTP host/)).not.toBeInTheDocument()
  await expect.element(dialog.getByLabelText(/^From address/)).not.toBeInTheDocument()
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ name: 'Test inbox', isDefault: false, config: { kind: 'inbox' } }])
})

test('says why a second Test inbox is refused', async () => {
  worker.use(
    list([]),
    handleSiteIntegrationsCreate(() =>
      problem(409, { detail: 'The workspace already has a Test inbox' }),
    ),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Test inbox' }).click()
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .element(screen.getByText('The workspace already has a Test inbox'))
    .toBeInTheDocument()
})

test('removes a provider', async () => {
  let removed = false
  worker.use(
    list([smtp]),
    handleSiteIntegrationsArchive(() => {
      removed = true
      return HttpResponse.json({ ...smtp, isDefault: false, archivedAt: '2026-02-01T00:00:00Z' })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()

  await expect.poll(() => removed).toBe(true)
})

test('a provider that live work uses is not removed and the work is named', async () => {
  worker.use(
    list([smtp]),
    handleSiteIntegrationsArchive(() =>
      problem(409, { usedBy: [{ kind: 'broadcast', id: '7', name: 'Summer sale teaser' }] }),
    ),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()

  await expect.element(screen.getByText('Failed to remove provider')).toBeInTheDocument()
  await expect.element(screen.getByText(/Broadcast "Summer sale teaser"/)).toBeInTheDocument()
})
test('shows an error alert when providers fail to load', async () => {
  worker.use(handleSiteIntegrationsList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Failed to load providers').first()).toBeInTheDocument()
})

test('reports a create failure', async () => {
  worker.use(
    list([]),
    handleSiteIntegrationsCreate(() => problem(422, { detail: 'bad host' })),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'SMTP' }).click()
  await dialog.getByLabelText(/^SMTP host/).fill('h')
  await dialog.getByLabelText(/^From address/).fill('a@b.co')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect.element(screen.getByText('Failed to save provider')).toBeInTheDocument()
})

const limited: SiteIntegrationResource = {
  ...smtp,
  maxPerSecond: 14,
  maxPerDay: 50000,
  providerLimit: {
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

test('warns when an Integration has no Provider limit', async () => {
  worker.use(list([smtp]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Primary SMTP has no Provider limit')).toBeInTheDocument()
  await expect.element(screen.getByText('Per second: Unlimited')).toBeInTheDocument()
})

test('warns when the provider send quota could not be read', async () => {
  const ses: SiteIntegrationResource = {
    ...limited,
    name: 'Amazon SES',
    providerLimit: { ...limited.providerLimit, warnings: ['providerQuotaUnavailable'] },
  }
  worker.use(list([ses]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect
    .element(screen.getByText("Amazon SES: the provider's send quota could not be read"))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText('Without the provider quota', { exact: false }))
    .toBeInTheDocument()
})

test('shows no quota warning when the provider quota is known', async () => {
  worker.use(list([limited]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Per second: 14')).toBeInTheDocument()
  await expect
    .element(screen.getByText('send quota could not be read', { exact: false }))
    .not.toBeInTheDocument()
})

test('shows the effective limit, its source and the 24-hour usage', async () => {
  worker.use(list([limited]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Per second: 14')).toBeInTheDocument()
  await expect.element(screen.getByText('Per 24 h: 50 000')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Sent in the last 24 h: 1 200 / 50 000'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Manual').first()).toBeInTheDocument()
  await expect.element(screen.getByText('has no Provider limit')).not.toBeInTheDocument()
})

test('connects a provider with a Provider limit', async () => {
  const bodies: unknown[] = []
  worker.use(list([]), create(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'Connect provider' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'SMTP' }).click()
  await dialog.getByLabelText(/^SMTP host/).fill('smtp.acme.com')
  await dialog.getByLabelText(/^From address/).fill('hi@acme.com')
  await dialog.getByRole('button', { name: 'Advanced settings' }).click()
  await dialog.getByLabelText('Max per second').fill('5')
  await dialog.getByLabelText('Max per 24 hours').fill('1000')
  await dialog.getByRole('button', { name: 'Connect provider' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ name: 'smtp.acme.com', maxPerSecond: 5, maxPerDay: 1000 }])
})

test('edits only the limits of an existing provider, and a blank clears one', async () => {
  const bodies: unknown[] = []
  worker.use(list([limited]), update(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Provider limit' }).click()
  const dialog = screen.getByRole('dialog')
  await expect.element(dialog.getByLabelText('Max per second')).toHaveValue('14')
  await dialog.getByLabelText('Max per second').fill('20')
  await dialog.getByLabelText('Max per 24 hours').clear()
  await dialog.getByRole('button', { name: 'Save limits' }).click()

  await expect.poll(() => bodies).toEqual([{ maxPerSecond: 20, maxPerDay: null }])
})

test('an SES-compatible service keeps both values: a blank is not saved', async () => {
  const postbox: SiteIntegrationResource = {
    ...limited,
    provider: 'ses',
    config: {
      kind: 'ses',
      region: 'ru-central1',
      from: 'hi@acme.com',
      endpoint: 'https://postbox.cloud.yandex.net',
    },
  }
  const bodies: unknown[] = []
  worker.use(list([postbox]), update(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Provider limit' }).click()
  const dialog = screen.getByRole('dialog')
  const perDay = dialog.getByLabelText(/^Max per 24 hours/)
  await expect.element(perDay).toBeRequired()
  await perDay.clear()
  await dialog.getByRole('button', { name: 'Save limits' }).click()

  await expect.element(perDay).toBeInvalid()
  await expect.element(dialog).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('makes another provider the default, and the default offers no such action', async () => {
  const backup: SiteIntegrationResource = {
    ...smtp,
    id: '2',
    name: 'Backup SMTP',
    isDefault: false,
  }
  const bodies: unknown[] = []
  worker.use(list([smtp, backup]), update(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Provider limit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Set as default' }).elements()).toHaveLength(0)
  await userEvent.keyboard('{Escape}')

  await screen.getByRole('button', { name: 'More actions' }).nth(1).click()
  await screen.getByRole('menuitem', { name: 'Set as default' }).click()

  await expect.poll(() => bodies).toEqual([{ isDefault: true }])
  await expect.element(screen.getByText('Default provider changed')).toBeInTheDocument()
})

test('a member without integrations:manage reads the providers but does not change them', async () => {
  worker.use(list([smtp]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={workspaceWith([])} />)

  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Connect provider' }))
    .not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Remove' })).not.toBeInTheDocument()
})

const generic = {
  name: 'generic',
  description: 'A body already in the Delivery notice shape, signed with Standard Webhooks.',
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
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

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
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

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
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Inbound' }).click()
  await screen.getByRole('button', { name: 'Regenerate URL' }).click()
  await screen
    .getByRole('dialog', { name: 'Regenerate the hook URL?' })
    .getByRole('button', { name: 'Regenerate URL' })
    .click()

  await expect.poll(() => calls).toEqual(['1'])
})
