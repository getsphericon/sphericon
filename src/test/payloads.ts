import type {
  BrandKitResource,
  CustomFieldConditionTests,
  RuleCondition,
  SegmentCondition,
  SiteIntegrationResource,
  SiteWorkspaceResource,
} from '../generated/site/types.gen.ts'

// Typed building blocks for the contract-typed `{ body }` responses of the generated MSW handlers.

export const TIMESTAMPS = { createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }

// page wraps items in the paginated list envelope every list endpoint returns.
export function page<T>(items: T[], totalItems = items.length) {
  return { items, page: 1, pageSize: 25, totalItems, totalPages: Math.ceil(totalItems / 25) }
}

// The empty Condition a Segment or a Rule version holds when it tests nothing: it matches every
// Contact.
export const EMPTY_CONDITION = {
  kind: 'group',
  combinator: 'and',
  entries: [],
} satisfies SegmentCondition & RuleCondition

// What a Custom field's Condition tests are to a page that does not offer them.
export const NO_CONDITION_TESTS: CustomFieldConditionTests = {
  text: [],
  number: [],
  moment: [],
  withinLast: false,
  presence: [],
}

// A brand color from its hex digits. A Brand kit's colors are data of the Workspace, not the
// app's own palette, so a test builds them here rather than writing color literals.
export const hex = (digits: string) => `#${digits}`

// The Brand kit of a Workspace that has not saved one: the defaults, `saved` false.
export const UNSAVED_BRAND_KIT: BrandKitResource = {
  saved: false,
  accentColor: hex('2563eb'),
  textColor: hex('111827'),
  backgroundColor: hex('ffffff'),
  headingFont: 'Arial, Helvetica, sans-serif',
  bodyFont: 'Arial, Helvetica, sans-serif',
  logoUrl: '',
  buttonTextColor: hex('ffffff'),
  buttonRadius: 4,
  footerText: '',
  updatedAt: null,
  revision: 1,
}

// The Workspace TestWs01 as the API lists it to its Owner, with a postal address (ADR 0156)
// unless the test says otherwise.
export function workspace(extra: Partial<SiteWorkspaceResource> = {}): SiteWorkspaceResource {
  return {
    id: 'TestWs01',
    name: 'Acme',
    collectKey: 'omck_test_key',
    revision: 1,
    postalAddress: '1 Test Street\nTestville',
    requireConfirmedOptIn: false,
    timezone: 'UTC',
    accountLinkAvailable: false,
    permissions: [],
    mcpUrl: 'https://sphericon.test',
    trackerUrl: 'https://t.sphericon.test',
    shortLinkLength: 35,
    smsMaxLength: { gsm7: 1600, ucs2: 700 },
    holds: [],
    role: 'owner',
    createdAt: TIMESTAMPS.createdAt,
    ...extra,
  }
}

// An enabled SMTP Integration of the email Channel, not the default, as the API returns it.
export function emailIntegration(
  id: string,
  name: string,
  extra: Partial<SiteIntegrationResource> = {},
): SiteIntegrationResource {
  return {
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
    revision: 1,
    hookUrl: `https://app.test/hooks/omik_${id}`,
    ...TIMESTAMPS,
    ...extra,
  }
}

// testId is an id of the contract's form (EntityId: 22 base62 characters) for test data: `test`
// and the number padded with zeros, so a test reads which row it names, the generated schemas
// accept it, and, never all digits like a real id, the router keeps it a string.
export const testId = (n: number) => 'test' + String(n).padStart(18, '0')

// A Step's or a path's id: a UUID in its lowercase text form, as the canvas and the server mint them.
export const stepId = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
