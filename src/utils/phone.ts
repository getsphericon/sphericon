import { parsePhoneNumberFromString } from 'libphonenumber-js/min'

// A Contact's phone, stored and sent by the API in E.164 (ADR 0075), in the readable international
// format: +14155550100 is shown as +1 415 555 0100. A value that does not parse is shown as it is.
export function formatPhone(phone: string): string {
  return parsePhoneNumberFromString(phone)?.formatInternational() ?? phone
}

// An SMS Destination erased with its Contact keeps only the number's hash (ADR 0021): the SHA-256 of
// the E.164 number in hex, where a number starts with `+`. Such a row is shown as an erased number,
// never as the hash.
const PHONE_HASH = /^[0-9a-f]{64}$/

export function isErasedNumber(destination: string): boolean {
  return PHONE_HASH.test(destination)
}
