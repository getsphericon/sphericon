import type { QueryClient } from '@tanstack/react-query'

import { siteUserGetMeOptions } from './generated/site/@tanstack/react-query.gen.ts'
import { hasErrorStatus } from './utils/apiErrors.ts'

// Whether someone is signed in, read fresh from the server: the guest pages (login, register)
// send a signed-in visitor on to the app. Only a 401 means "not signed in".
export async function isSignedIn(queryClient: QueryClient) {
  try {
    await queryClient.query({ ...siteUserGetMeOptions(), staleTime: 0 })
    return true
  } catch (error) {
    if (hasErrorStatus(error, [401])) {
      return false
    }
    throw error
  }
}
