import type { SiteContactResource } from '../generated/site/types.gen.ts'
import { formatPhone } from './phone.ts'

type Named = Pick<SiteContactResource, 'firstName' | 'lastName'>
type Identified = Pick<SiteContactResource, 'email' | 'subjectId' | 'phone'>

// A Contact's first and last name, or '' when it has neither.
export function contactFullName(contact: Named): string {
  return [contact.firstName, contact.lastName].filter(Boolean).join(' ')
}

// The first identifier a Contact has (email, subject id, phone), the phone in its readable
// international format; undefined when it has none.
export function contactIdentifier(contact: Identified): string | undefined {
  return (
    contact.email ?? contact.subjectId ?? (contact.phone ? formatPhone(contact.phone) : undefined)
  )
}

// What a Contact's avatar is named after: the full name, else the first identifier it has.
export function contactName(contact: Named & Identified): string {
  return contactFullName(contact) || (contactIdentifier(contact) ?? '')
}
