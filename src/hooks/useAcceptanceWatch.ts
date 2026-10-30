import { useQueryClient } from '@tanstack/react-query'
import type { MutationCacheNotifyEvent, QueryCacheNotifyEvent } from '@tanstack/react-query'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { useEffect } from 'react'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as acceptanceRoute } from '../routes/acceptance.tsx'
import { isAcceptancePendingError } from '../utils/apiErrors.ts'

// useAcceptanceWatch sends a User already in the app to the accept-or-sign-out screen once the
// server refuses one of their requests with 403 acceptance_pending (ADR 0062): a new version of a
// required item was published after the app loaded the User. The cached User is dropped, so the
// screen's guard reads the pending acceptances afresh, and the page they were on is resumed once
// they accept.
export function useAcceptanceWatch() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const router = useRouter()
  useEffect(() => {
    const stop = async () => {
      queryClient.removeQueries({ queryKey: siteUserGetMeQueryKey() })
      await navigate({
        to: acceptanceRoute.to,
        search: { redirect: router.state.location.href },
      })
    }
    // Only a refusal as it arrives counts: a query keeps its last error while it fetches again,
    // and that stale refusal must not send back a User who has accepted since. A query and a
    // mutation are refused alike.
    const onRefusal = (event: QueryCacheNotifyEvent | MutationCacheNotifyEvent) => {
      if (
        event.type === 'updated' &&
        event.action.type === 'error' &&
        isAcceptancePendingError(event.action.error)
      ) {
        void stop()
      }
    }
    const unsubscribeQueries = queryClient.getQueryCache().subscribe(onRefusal)
    const unsubscribeMutations = queryClient.getMutationCache().subscribe(onRefusal)
    return () => {
      unsubscribeQueries()
      unsubscribeMutations()
    }
  }, [queryClient, navigate, router])
}
