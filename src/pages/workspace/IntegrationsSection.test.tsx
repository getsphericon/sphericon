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
  handleSiteIntegrationsUnarchive,
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

test('warns once about all the Integrations that share a warning', async () => {
  const backup: SiteIntegrationResource = {
    ...smtp,
    id: '2',
    name: 'Backup SMTP',
    isDefault: false,
  }
  worker.use(list([smtp, backup]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect
    .element(screen.getByText('Primary SMTP and Backup SMTP have no Provider limit'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText('Without a limit, mail is not paced', { exact: false }))
    .toBeInTheDocument()
  expect(
    screen.getByText('Without a limit, mail is not paced', { exact: false }).all(),
  ).toHaveLength(1)
  expect(screen.getByText('No limit', { exact: true }).all()).toHaveLength(2)
})

test('puts a stopped account above the other warnings', async () => {
  const stopped: SiteIntegrationResource = {
    ...limited,
    id: '3',
    name: 'Amazon SES',
    providerLimit: { ...limited.providerLimit, warnings: ['providerStopped'] },
  }
  worker.use(list([smtp, stopped]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  const titles = screen.getByText(/the provider has stopped this account|has no Provider limit/)
  await expect
    .element(titles.first())
    .toHaveTextContent('Amazon SES: the provider has stopped this account')
  await expect.element(titles.nth(1)).toHaveTextContent('Primary SMTP has no Provider limit')
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

test.each([
  [
    'providerStopped' as const,
    'Amazon SES: the provider has stopped this account',
    'mail through it is on hold',
  ],
  [
    'providerProbation' as const,
    'Amazon SES: the provider has put this account under review',
    'Mail still goes out',
  ],
  [
    'providerSandbox' as const,
    "Amazon SES: the account is in the provider's sandbox",
    'only to verified addresses',
  ],
])('warns about the provider account standing: %s', async (warning, title, body) => {
  const ses: SiteIntegrationResource = {
    ...limited,
    name: 'Amazon SES',
    providerLimit: { ...limited.providerLimit, warnings: [warning] },
  }
  worker.use(list([ses]))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText(title)).toBeInTheDocument()
  await expect.element(screen.getByText(body, { exact: false })).toBeInTheDocument()
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

// The built-in Presets as the server lists them (internal/preset/builtin), and a custom one with
// params that are not strings.
const generic = {
  name: 'generic',
  description: 'A body already in the Delivery notice shape, signed with Standard Webhooks.',
  paramsSchema: {
    type: 'object',
    required: ['secret'],
    additionalProperties: false,
    properties: {
      secret: {
        type: 'string',
        title: 'Signing secret',
        description: 'The whsec_ secret the sender signs with.',
        writeOnly: true,
      },
    },
  },
}
const ses = {
  name: 'ses',
  description: 'Amazon SES events through SNS.',
  paramsSchema: {
    type: 'object',
    required: ['topic_arn'],
    additionalProperties: false,
    properties: {
      topic_arn: {
        type: 'string',
        title: 'SNS topic ARN',
        description: 'The one SNS topic your SES events are published to.',
        pattern: '^arn:aws[a-z-]*:sns:[a-z0-9-]+:[0-9]+:[A-Za-z0-9_-]+$',
      },
    },
  },
}
const postmark = {
  name: 'postmark',
  description: 'Postmark webhooks behind basic auth.',
  paramsSchema: {
    type: 'object',
    required: ['user', 'secret'],
    additionalProperties: false,
    properties: {
      user: { type: 'string', title: 'Basic auth user name' },
      secret: { type: 'string', title: 'Basic auth password', writeOnly: true },
    },
  },
}
const mailgun = {
  name: 'mailgun',
  description: 'Mailgun webhooks.',
  paramsSchema: {
    type: 'object',
    required: ['signing_key'],
    additionalProperties: false,
    properties: {
      signing_key: { type: 'string', title: 'HTTP webhook signing key', writeOnly: true },
    },
  },
}
const custom = {
  name: 'acme-relay',
  description: 'A custom Preset.',
  paramsSchema: {
    type: 'object',
    required: ['retries'],
    properties: {
      retries: { type: 'integer', title: 'Retries', minimum: 0 },
      strict: { type: 'boolean', title: 'Strict mode' },
      relay: { type: 'string', title: 'Relay address', format: 'ipv4' },
    },
  },
}
const PRESETS = [generic, ses, postmark, mailgun, custom]

const withPreset: SiteIntegrationResource = {
  ...smtp,
  preset: 'generic',
  params: { secret: '********' },
  secretsRetiring: ['secret'],
}

// Opens the inbound settings of the only provider listed.
async function openInbound(screen: Awaited<ReturnType<typeof renderWithRouter>>['screen']) {
  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Inbound' }).click()
}

async function choosePreset(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  name: string,
) {
  await screen.getByRole('combobox', { name: 'Preset' }).click()
  await screen.getByRole('option', { name }).click()
}

const updates = (bodies: unknown[], answer = withPreset) =>
  handleSiteIntegrationsUpdate(async ({ request }) => {
    bodies.push(await request.json())
    return HttpResponse.json(answer)
  })

test('shows the hook URL and chooses a Preset, its params as fields', async () => {
  const bodies: unknown[] = []
  worker.use(list([smtp]), handleSitePresetsList({ body: PRESETS }), updates(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await openInbound(screen)
  await expect.element(screen.getByText('https://app.test/hooks/omik_one')).toBeInTheDocument()
  await choosePreset(screen, 'generic')
  await screen.getByLabelText(/^Signing secret/).fill('whsec_abc')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ preset: 'generic', params: { secret: 'whsec_abc' } }])
  await expect.element(screen.getByRole('textbox', { name: /^Params/ })).not.toBeInTheDocument()
})

test('every built-in Preset and a custom one render their params as fields', async () => {
  worker.use(list([smtp]), handleSitePresetsList({ body: PRESETS }))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)
  await openInbound(screen)

  await choosePreset(screen, 'ses')
  await expect.element(screen.getByLabelText(/^SNS topic ARN/)).toBeInTheDocument()
  await choosePreset(screen, 'postmark')
  await expect.element(screen.getByLabelText(/^Basic auth user name/)).toBeInTheDocument()
  await expect
    .element(screen.getByLabelText(/^Basic auth password/))
    .toHaveAttribute('type', 'password')
  await choosePreset(screen, 'mailgun')
  await expect.element(screen.getByLabelText(/^HTTP webhook signing key/)).toBeInTheDocument()
  await choosePreset(screen, 'acme-relay')
  await expect.element(screen.getByLabelText(/^Retries/)).toBeInTheDocument()
  await expect.element(screen.getByRole('checkbox', { name: /Strict mode/ })).toBeInTheDocument()
})

test('a value the schema refuses is shown on its field and nothing is sent', async () => {
  const bodies: unknown[] = []
  worker.use(list([smtp]), handleSitePresetsList({ body: PRESETS }), updates(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)
  await openInbound(screen)

  await choosePreset(screen, 'ses')
  await screen.getByLabelText(/^SNS topic ARN/).fill('not-an-arn')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText(/must match pattern/).first()).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('a value off its format is shown on its field and nothing is sent', async () => {
  const bodies: unknown[] = []
  worker.use(list([smtp]), handleSitePresetsList({ body: PRESETS }), updates(bodies))
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)
  await openInbound(screen)

  await choosePreset(screen, 'acme-relay')
  await screen.getByLabelText(/^Retries/).fill('1')
  await screen.getByLabelText(/^Relay address/).fill('not an address')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText(/must match format "ipv4"/).first()).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test("the server's refusal of a param lands on its field", async () => {
  worker.use(
    list([smtp]),
    handleSitePresetsList({ body: PRESETS }),
    handleSiteIntegrationsUpdate(() =>
      problem(422, { errors: { 'params.user': ['the user name is taken'] } }),
    ),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)
  await openInbound(screen)

  await choosePreset(screen, 'postmark')
  await screen.getByLabelText(/^Basic auth user name/).fill('hooks')
  await screen.getByLabelText(/^Basic auth password/).fill('pw')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('the user name is taken').first()).toBeInTheDocument()
})

test('a saved secret shows blank, a blank keeps it and a new value starts a rotation', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([{ ...withPreset, secretsRetiring: [] }]),
    handleSitePresetsList({ body: PRESETS }),
    updates(bodies),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)
  await openInbound(screen)

  const secret = screen.getByLabelText(/^Signing secret/)
  await expect.element(secret).toHaveValue('')
  await expect.element(secret).toHaveAttribute('placeholder', 'Stored. Leave blank to keep it.')
  await expect.element(screen.getByText(/A new value starts a rotation/)).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Save' }).click()
  await expect.poll(() => bodies).toEqual([{ preset: 'generic', params: {} }])

  await secret.fill('whsec_new')
  await screen.getByRole('button', { name: 'Save' }).click()
  await expect
    .poll(() => bodies)
    .toEqual([
      { preset: 'generic', params: {} },
      { preset: 'generic', params: { secret: 'whsec_new' } },
    ])
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

test('switches to the archived providers and restores one', async () => {
  const restored: string[] = []
  const archivedSmtp = {
    ...smtp,
    id: '9',
    name: 'Old SMTP',
    isDefault: false,
    archivedAt: '2026-02-01T00:00:00Z',
  }
  worker.use(
    handleSiteIntegrationsList(({ request }) =>
      HttpResponse.json(
        new URL(request.url).searchParams.get('archived') === 'true' ? [archivedSmtp] : [smtp],
      ),
    ),
    handleSiteIntegrationsUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json({ ...archivedSmtp, archivedAt: null })
    }),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await expect.element(screen.getByText('Primary SMTP')).toBeInTheDocument()
  await screen.getByText('Archived', { exact: true }).click()
  await expect.element(screen.getByText('Old SMTP')).toBeInTheDocument()
  await expect.element(screen.getByText('Primary SMTP')).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).click()
  await expect.element(screen.getByRole('menuitem', { name: 'Remove' })).not.toBeInTheDocument()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['9'])
})

test('an empty archive says so', async () => {
  worker.use(
    handleSiteIntegrationsList(({ request }) =>
      HttpResponse.json(new URL(request.url).searchParams.get('archived') === 'true' ? [] : [smtp]),
    ),
  )
  const { screen } = await renderWithRouter(<IntegrationsSection workspace={manager} />)

  await screen.getByText('Archived', { exact: true }).click()

  await expect.element(screen.getByText('No archived providers')).toBeInTheDocument()
})
