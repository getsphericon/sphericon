export const options = [
  { value: 'contacts:read', label: 'Read' },
  { value: 'contacts:write', label: 'Write' },
  { value: 'events:read', label: 'Events' },
]
export interface Button {
  label: string
  url?: string | null
}
export const buttons = z.array(z.object({ label: z.string(), url: z.string() }))
export const query = { query: { pageSize: 250 } }
export const shown = { query: { pageSize: 250, page: 1 } }
