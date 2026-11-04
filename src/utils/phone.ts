import { parsePhoneNumberFromString } from 'libphonenumber-js/min'

// A Contact's phone, stored and sent by the API in E.164 (ADR 0075), in the readable international
// format: +14155550100 is shown as +1 415 555 0100. A value that does not parse is shown as it is.
export function formatPhone(phone: string): string {
  return parsePhoneNumberFromString(phone)?.formatInternational() ?? phone
}
