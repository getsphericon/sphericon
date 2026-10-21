import { z } from 'zod'

// A search param that carries a link the server built: an absolute http(s) URL, anything else
// (a `javascript:` or `data:` URL, plain text) is dropped so a page never renders it as an href.
export const httpUrlParam = z
  .url({ protocol: /^https?$/ })
  .optional()
  .catch(undefined)
