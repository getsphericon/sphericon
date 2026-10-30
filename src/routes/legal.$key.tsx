import { createFileRoute } from '@tanstack/react-router'

// The public page of a Sign-up item's Legal document (ADR 0062): no sign-in.
export const Route = createFileRoute('/legal/$key')()
