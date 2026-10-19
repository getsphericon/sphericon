import { useQuery } from '@tanstack/react-query'

import { siteInstanceGetStateOptions } from '../generated/site/@tanstack/react-query.gen.ts'

// useInstanceState reads the Instance state (ADR 0032). Only the Owners of the oldest
// Workspace may: everyone else gets a 403, so `data` stays undefined for them and nothing
// Instance-related is shown.
export function useInstanceState() {
  return useQuery({ ...siteInstanceGetStateOptions(), retry: false })
}
