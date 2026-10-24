import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'

// The window every analytics report reads: the section's `range` search param.
export function useAnalyticsRange() {
  return analyticsRoute.useSearch().range
}
