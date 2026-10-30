import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { requireUser } from '../currentUser.ts'

// Where the mailed deletion link of a User with no password lands (User deletion, ADR 0050). It
// asks only for a signed-in User, so a User stopped at the acceptance screen (ADR 0062) can still
// finish deleting their account.
export const Route = createFileRoute('/delete-account')({
  validateSearch: z.object({ token: z.string().catch('') }),
  beforeLoad: async ({ context, location }) => {
    await requireUser(context.queryClient, location.href)
  },
})
