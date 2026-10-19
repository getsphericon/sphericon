import type { SiteBrandKitResource } from '../generated/site/types.gen.ts'

// Typed building blocks for the contract-typed `{ body }` responses of the generated MSW handlers.

export const TIMESTAMPS = { createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }

// page wraps items in the paginated list envelope every list endpoint returns.
export function page<T>(items: T[], totalItems = items.length) {
  return { items, page: 1, pageSize: 25, totalItems, totalPages: Math.ceil(totalItems / 25) }
}

// A brand color from its hex digits. A Brand kit's colors are data of the Workspace, not the
// app's own palette, so a test builds them here rather than writing color literals.
export const hex = (digits: string) => `#${digits}`

// The Brand kit of a Workspace that has not saved one: the defaults, `saved` false.
export const UNSAVED_BRAND_KIT: SiteBrandKitResource = {
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
}
