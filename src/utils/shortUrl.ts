const DEFAULT_MAX = 40

// shortUrl is a link as a feed row shows it: without its scheme, and a long one cut to `max`
// characters with an ellipsis. The full link goes on hover.
export function shortUrl(url: string, max = DEFAULT_MAX): string {
  const bare = url.replace(/^https?:\/\//i, '')
  return bare.length <= max ? bare : `${bare.slice(0, max - 1)}…`
}
