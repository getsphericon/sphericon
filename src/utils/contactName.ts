import type { SiteContactResource } from '../generated/site/types.gen.ts'

type Named = Pick<SiteContactResource, 'firstName' | 'lastName'>

// A Contact's first and last name, or '' when it has neither.
export function contactFullName(contact: Named): string {
  return [contact.firstName, contact.lastName].filter(Boolean).join(' ')
}

// What a Contact's avatar is named after: the full name, else the first identifier it has.
export function contactName(
  contact: Named & Pick<SiteContactResource, 'email' | 'subjectId' | 'phone'>,
): string {
  return contactFullName(contact) || (contact.email ?? contact.subjectId ?? contact.phone ?? '')
}
