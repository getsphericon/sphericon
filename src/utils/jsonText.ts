import * as z from 'zod'

// Form state is strings, so a form holds a structured value as its JSON text; this codec is the
// one place the text meets the JSON value. A blank or unreadable text is an issue.
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
