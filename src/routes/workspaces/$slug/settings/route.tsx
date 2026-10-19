import { createFileRoute } from '@tanstack/react-router'

// The settings area: its pages render under the settings sidebar (see WorkspaceLayout).
export const Route = createFileRoute('/workspaces/$slug/settings')({})
