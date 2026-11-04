// The SMS counter (ADR 0078): what the editor shows as the marketer types. internal/smsrender
// counts the same way for the save, and both agree on internal/smsrender/testdata/parts.json.

type SmsEncoding = 'GSM-7' | 'UCS-2'

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

// The longest text the SMS provider sends, in characters of each encoding as measureSms counts
// them: the Workspace's `smsMaxLength`.
export interface SmsLengthLimit {
  gsm7: number
  ucs2: number
}

// The most characters size's encoding allows under limit.
export function smsLimitOf(size: SmsSize, limit: SmsLengthLimit): number {
  return size.encoding === 'UCS-2' ? limit.ucs2 : limit.gsm7
}

// Whether a text of size is longer than the provider sends in its encoding: the editor flags it,
// as the save refuses it.
export function tooLongForSms(size: SmsSize, limit: SmsLengthLimit): boolean {
  return size.characters > smsLimitOf(size, limit)
}

// A link as a phone finds it: an absolute http(s) URL up to the next whitespace, a Liquid tag inside
// it part of it, a lone `{` its end. Whitespace is spelled out, as in internal/smsrender/links.go:
// the two regexp engines differ on what `\s` covers.
const LINK = /https?:\/\/(?:\{\{.*?\}\}|\{%.*?%\}|[^\t\n\f\r {])+/g
// The punctuation that ends a sentence rather than a link.
const TRAILING = '.,;:!?\'"'

const count = (s: string, c: string) => s.split(c).length - 1

// The length of candidate once trailing sentence marks, and a `)` with no `(` inside the link, are cut.
function trim(candidate: string): number {
  let end = candidate.length
  while (end > 0) {
    const c = candidate.charAt(end - 1)
    const head = candidate.slice(0, end)
    if (TRAILING.includes(c) || (c === ')' && count(head, '(') < count(head, ')'))) {
      end--
    } else {
      break
    }
  }
  return end
}

// smsLinks are the links of text, in order: what the send turns into short tracked links (ADR 0083).
export function smsLinks(text: string): string[] {
  const out: string[] = []
  for (const m of text.matchAll(LINK)) {
    const end = trim(m[0])
    if (end > m[0].indexOf('://') + 3) {
      out.push(m[0].slice(0, end))
    }
  }
  return out
}

// measureSms counts text as it goes out, each link a short link of linkLength characters (the
// Workspace's `shortLinkLength`); 0, a text whose links are not tracked, counts them as written.
// Merge tags count as their own characters. GSM-7 is 160 characters in one part or 153 per part;
// one character outside it makes the whole text UCS-2, 70 in one part or 67 per part.
export function measureSms(written: string, linkLength = 0): SmsSize {
  const short = 'x'.repeat(linkLength)
  const text =
    linkLength > 0
      ? written.replace(LINK, (m) => {
          const end = trim(m)
          return end > m.indexOf('://') + 3 ? short + m.slice(end) : m
        })
      : written
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
