// The SMS counter (ADR 0078): what the editor shows as the marketer types. internal/smsrender
// counts the same way for the save, and both agree on internal/smsrender/testdata/parts.json.

export type SmsEncoding = 'GSM-7' | 'UCS-2'

// A text as an SMS counts it: its encoding, its characters in that encoding (a GSM-7 extension
// character is two, a UCS-2 character beyond the BMP two UTF-16 code units) and its parts.
export interface SmsSize {
  encoding: SmsEncoding
  characters: number
  parts: number
}

// The GSM 03.38 default alphabet, and the extension characters sent as an escape and a second
// septet.
const GSM7_BASIC = new Set(
  '@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !"#¤%&\'()*+,-./0123456789:;<=>?' +
    '¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà',
)
const GSM7_EXTENSION = new Set('\f^{}\\[~]|€')

// How many parts n characters take: one up to single, else n split into parts of each.
function parts(n: number, single: number, each: number): number {
  return n <= single ? Math.min(n, 1) : Math.ceil(n / each)
}

// measureSms counts text as written: merge tags count as their own characters. GSM-7 is 160
// characters in one part or 153 per part; one character outside it makes the whole text UCS-2,
// 70 in one part or 67 per part.
export function measureSms(text: string): SmsSize {
  let septets = 0
  for (const char of text) {
    if (GSM7_BASIC.has(char)) {
      septets += 1
    } else if (GSM7_EXTENSION.has(char)) {
      septets += 2
    } else {
      return { encoding: 'UCS-2', characters: text.length, parts: parts(text.length, 70, 67) }
    }
  }
  return { encoding: 'GSM-7', characters: septets, parts: parts(septets, 160, 153) }
}
