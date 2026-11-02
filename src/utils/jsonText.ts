import * as z from 'zod'

// Text editors and library exports meet structured JSON here. A blank or unreadable text
// is an issue; resource forms hold structured values directly.
export const jsonText = z.codec(z.string(), z.json(), {
  decode: (text, ctx) => {
    try {
      return z.json().parse(JSON.parse(text))
    } catch {
      ctx.issues.push({ code: 'invalid_format', format: 'json', input: text })
      return z.NEVER
    }
  },
  encode: (value) => JSON.stringify(value),
})
