import { expect, test } from 'vitest'

import {
  handleSiteAuditFacets,
  handleSiteAuditGetRetention,
  handleSiteAuditList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as auditLogRoute } from '../../../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { AuditLogPage } from './audit-log.tsx'

const AUDIT_LOG_ROUTE = routeMount(auditLogRoute, { workspaceId: 'TestWs01' })

const workspace = {
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://mcp.sphericon.test/',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

function serveTheLog() {
  worker.use(
    handleSiteAuditList({
      body: {
        items: [
          {
            id: '2',
            occurredAt: '2026-01-02T03:04:05Z',
            actor: { kind: 'user', id: '1', name: 'John' },
            action: 'membership.update',
            target: { type: 'membership', id: '3', name: 'Mary' },
            diff: { role: { from: 'member', to: 'admin' } },
          },
        ],
      },
    }),
    handleSiteAuditFacets({ body: { actors: [], actions: [], targetTypes: [] } }),
    handleSiteAuditGetRetention({ body: { retentionDays: 90 } }),
  )
}

test('a member who reads the log sees it with its export', async () => {
  serveTheLog()
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, permissions: ['audit:read'] }] }))

  const { screen } = await renderWithRouter(<AuditLogPage />, AUDIT_LOG_ROUTE)

  await expect.element(screen.getByText('John')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Export CSV' })).toBeInTheDocument()
})

test('a member without audit:read sees the title and neither the log nor its export', async () => {
  serveTheLog()
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' }] }))

  const { screen } = await renderWithRouter(<AuditLogPage />, AUDIT_LOG_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'Audit log' })).toBeInTheDocument()
  await expect.element(screen.getByText('John')).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Export CSV' })).not.toBeInTheDocument()
})
