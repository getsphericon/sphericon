export const options = [
  { value: 'red', label: 'Red' },
  { value: 'green', label: 'Green' },
]
export interface Link {
  label: string
  url: string
  external: boolean
}
// contract-copy: the canvas holds a button as its own node data, not the contract's value
export const node = z.object({ label: z.string(), url: z.string() })
export const query = { pageSize: 10 }
// Each field is the contract's own schema, trimmed for a form: derived, not copied.
export const form = z.object({ label: trimmed(zButton.shape.label), url: trimmed(zButton.shape.url) })
export function page() {
  return { items: [], page: 1, pageSize: 250, totalItems: 0, totalPages: 1 }
}
