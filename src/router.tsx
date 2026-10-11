import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
} from '@tanstack/react-router'
import { z } from 'zod'

import App from './App.tsx'
import { siteWorkspacesList } from './generated/site/sdk.gen.ts'
import type { SiteWorkspaceResource } from './generated/site/types.gen.ts'
import { AccountLayout } from './layouts/AccountLayout.tsx'
import { WorkspaceLayout } from './layouts/WorkspaceLayout.tsx'
import { ProfilePage } from './routes/account/profile.tsx'
import { SecurityPage } from './routes/account/security.tsx'
import { ConfirmEmailChangePage } from './routes/auth/confirm-email-change.tsx'
import { ForgotPasswordPage } from './routes/auth/forgot-password.tsx'
import { LoginPage } from './routes/auth/login.tsx'
import { RegisterPage } from './routes/auth/register.tsx'
import { ResetPasswordPage } from './routes/auth/reset-password.tsx'
import { VerifyEmailPage } from './routes/auth/verify-email.tsx'
import { AutomationCreatePage } from './routes/automations/create.tsx'
import { AutomationEditPage } from './routes/automations/edit.tsx'
import { AutomationsListPage } from './routes/automations/list.tsx'
import { BroadcastsListPage } from './routes/broadcasts/list.tsx'
import { BroadcastReportPage } from './routes/broadcasts/report.tsx'
import { BroadcastCreatePage, BroadcastEditPage } from './routes/broadcasts/resource.tsx'
import { ConfirmSubscriptionPage } from './routes/confirm.tsx'
import { ContactDetailPage } from './routes/contacts/detail.tsx'
import { ContactsListPage } from './routes/contacts/list.tsx'
import { ContactCreatePage, ContactEditPage } from './routes/contacts/resource.tsx'
import { AcceptInvitationPage } from './routes/invitations/accept.tsx'
import { OAuthConsentPage } from './routes/oauth/consent.tsx'
import { SegmentsListPage } from './routes/segments/list.tsx'
import { SegmentCreatePage, SegmentEditPage } from './routes/segments/resource.tsx'
import { TemplatesListPage } from './routes/templates/list.tsx'
import { TemplateCreatePage, TemplateEditPage } from './routes/templates/resource.tsx'
import { TransactionalEmailsListPage } from './routes/transactional-emails/list.tsx'
import { UnsubscribePage } from './routes/unsubscribe.tsx'
import { UnsubscribedPage } from './routes/unsubscribed.tsx'
import { ActivityPage } from './routes/workspace/activity.tsx'
import { auditFilterSchema } from './routes/workspace/auditFilter.ts'
import { OverviewPage } from './routes/workspace/overview.tsx'
import { SettingsPage } from './routes/workspace/settings.tsx'

// fetchWorkspaces loads the authenticated user's workspaces via the generated
// client, redirecting to /login on 401. Used by route guards to authenticate
// and resolve the active workspace slug.
async function fetchWorkspaces(): Promise<SiteWorkspaceResource[]> {
  const { data, response } = await siteWorkspacesList()
  if (response?.status === 401) {
    throw redirect({ to: loginRoute.to })
  }
  return data ?? []
}

const rootRoute = createRootRoute({
  component: App,
})

export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: async () => {
    const workspaces = await fetchWorkspaces()
    const first = workspaces[0]
    if (!first) {
      throw redirect({ to: loginRoute.to })
    }
    throw redirect({ to: overviewRoute.to, params: { slug: first.slug } })
  },
})

// Workspace-scoped layout: authenticates and verifies the slug is one the user owns.
export const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workspaces/$slug',
  beforeLoad: async ({ params }) => {
    const workspaces = await fetchWorkspaces()
    const current = workspaces.find((w) => w.slug === params.slug)
    if (!current) {
      const fallback = workspaces[0]
      if (!fallback) {
        throw redirect({ to: loginRoute.to })
      }
      throw redirect({ to: overviewRoute.to, params: { slug: fallback.slug } })
    }
    return { workspaces }
  },
  component: WorkspaceLayout,
})

// Index route for /w/$slug — the workspace landing.
export const overviewRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/',
  component: OverviewPage,
})

export const contactsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'contacts',
  component: ContactsListPage,
})

export const segmentsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'segments',
  component: SegmentsListPage,
})

export const segmentsCreateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'segments/new',
  component: SegmentCreatePage,
})

export const segmentsEditRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'segments/$segmentId/edit',
  component: SegmentEditPage,
})

export const broadcastsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'broadcasts',
  component: BroadcastsListPage,
})

export const broadcastsCreateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'broadcasts/new',
  component: BroadcastCreatePage,
})

export const broadcastsEditRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'broadcasts/$broadcastId/edit',
  component: BroadcastEditPage,
})

export const broadcastsReportRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'broadcasts/$broadcastId/report',
  component: BroadcastReportPage,
})

export const transactionalEmailsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'transactional-emails',
  component: TransactionalEmailsListPage,
})

export const templatesRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'templates',
  component: TemplatesListPage,
})

export const templatesCreateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'templates/new',
  component: TemplateCreatePage,
})

export const templatesEditRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'templates/$templateId/edit',
  component: TemplateEditPage,
})

export const automationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'automations',
  component: AutomationsListPage,
})

export const automationsCreateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'automations/new',
  component: AutomationCreatePage,
})

export const automationsEditRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'automations/$automationId/edit',
  component: AutomationEditPage,
})

export const activityRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'activity',
  component: ActivityPage,
})

export const settingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'settings',
  // The Audit log filter, so a Change history link opens the log pre-filtered.
  validateSearch: auditFilterSchema,
  component: SettingsPage,
})

export const contactsCreateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'contacts/new',
  component: ContactCreatePage,
})

export const contactsEditRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'contacts/$contactId/edit',
  component: ContactEditPage,
})

export const contactsDetailRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: 'contacts/$contactId',
  component: ContactDetailPage,
})

export const registerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/register',
  component: RegisterPage,
})

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  // `redirect` is a same-origin path to return to after signing in.
  validateSearch: z.object({
    redirect: z
      .string()
      .startsWith('/')
      .refine((path) => !path.startsWith('//'))
      .optional()
      .catch(undefined),
  }),
  component: LoginPage,
})

// Public self-service auth pages (no auth guard): reached from emailed links.
export const forgotPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/forgot-password',
  component: ForgotPasswordPage,
})

export const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reset-password',
  validateSearch: z.object({ token: z.string().catch('') }),
  component: ResetPasswordPage,
})

export const verifyEmailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/verify-email',
  validateSearch: z.object({ token: z.string().catch('') }),
  component: VerifyEmailPage,
})

export const confirmEmailChangeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/confirm-email-change',
  validateSearch: z.object({ token: z.string().catch('') }),
  component: ConfirmEmailChangePage,
})

// Public double opt-in confirmation page (no auth): the GET /e/confirm/{token}
// endpoint redirects here (ADR 0013 — GET records nothing). `token` is sent through
// the site API on confirm to record the confirmation; an expired/invalid
// link arrives with `expired=1` and no token, so the page offers sign-up-again.
export const confirmRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/confirm',
  validateSearch: z.object({ token: z.string().optional(), expired: z.string().optional() }),
  component: ConfirmSubscriptionPage,
})

// Public unsubscribe confirm page (no auth): the GET /e/u/{token} endpoint
// redirects here (RFC 8058 / ADR 0012 — GET records nothing). `token` is sent through
// the site API on confirm to perform the opt-out; `all` is the optional
// "unsubscribe from everything" escalation URL the backend supplies.
export const unsubscribeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/unsubscribe',
  validateSearch: z.object({ token: z.string(), all: z.string().optional() }),
  component: UnsubscribePage,
})

// Public unsubscribe "done" page (no auth): shown after the opt-out is recorded.
// `all` is the optional "unsubscribe from everything" escalation URL.
export const unsubscribedRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/unsubscribed',
  validateSearch: z.object({ all: z.string().optional() }),
  component: UnsubscribedPage,
})

// Public invite acceptance (no auth): opened from the invite link. The token in
// the path is the authorization; the page creates or attaches the user on accept.
export const acceptInvitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/invitations/$token',
  component: AcceptInvitationPage,
})

// OAuth consent screen: /oauth/authorize (Go) redirects the browser here with the
// MCP client's authorization request. Signed-out users sign in first and come back.
export const oauthConsentRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/oauth/consent',
  validateSearch: z.object({
    client_id: z.string().catch(''),
    redirect_uri: z.string().catch(''),
    code_challenge: z.string().catch(''),
    state: z.string().catch(''),
    scope: z.string().catch(''),
  }),
  beforeLoad: async ({ location }) => {
    const { response } = await siteWorkspacesList()
    if (response?.status === 401) {
      throw redirect({ to: loginRoute.to, search: { redirect: location.href } })
    }
  },
  component: OAuthConsentPage,
})

// Account layout: workspace-independent settings. Authenticates via the same
// workspace fetch (redirects to /login on 401) but renders its own shell.
export const accountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/account',
  beforeLoad: async () => {
    await fetchWorkspaces()
  },
  component: AccountLayout,
})

export const profileRoute = createRoute({
  getParentRoute: () => accountRoute,
  path: '/',
  component: ProfilePage,
})

export const securityRoute = createRoute({
  getParentRoute: () => accountRoute,
  path: '/security',
  component: SecurityPage,
})

// The Operator console (ADR 0026): a lazily loaded subtree. It lives at /console, not
// /operator, because the Go server owns the /operator/* API prefix. Nothing under it
// (components, the generated Operator client) is fetched until the first visit.
export const consoleRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console',
})

export const consoleLoginRoute = createRoute({
  getParentRoute: () => consoleRoute,
  path: '/login',
  component: lazyRouteComponent(() => import('./console/login.tsx'), 'ConsoleLoginPage'),
})

// Guarded by the Operator session: anything but a signed-in Operator (no cookie, an
// expired one, or no license, where the whole surface answers 404) goes to the login.
export const consoleAuthedRoute = createRoute({
  getParentRoute: () => consoleRoute,
  id: 'authed',
  beforeLoad: async () => {
    const { operatorMeGet } = await import('./generated/operator/sdk.gen.ts')
    const { data } = await operatorMeGet()
    if (!data) {
      throw redirect({ to: consoleLoginRoute.to })
    }
    return { operator: data }
  },
  component: lazyRouteComponent(() => import('./console/layout.tsx'), 'ConsoleLayout'),
})

// The console's home is the Workspace list.
export const consoleHomeRoute = createRoute({
  getParentRoute: () => consoleAuthedRoute,
  path: '/',
  component: lazyRouteComponent(() => import('./console/workspaces.tsx'), 'ConsoleWorkspacesPage'),
})

export const consoleWorkspaceRoute = createRoute({
  getParentRoute: () => consoleAuthedRoute,
  path: 'workspaces/$workspaceId',
  component: lazyRouteComponent(() => import('./console/workspace.tsx'), 'ConsoleWorkspacePage'),
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  registerRoute,
  forgotPasswordRoute,
  resetPasswordRoute,
  verifyEmailRoute,
  confirmEmailChangeRoute,
  confirmRoute,
  unsubscribeRoute,
  unsubscribedRoute,
  acceptInvitationRoute,
  oauthConsentRoute,
  accountRoute.addChildren([profileRoute, securityRoute]),
  consoleRoute.addChildren([
    consoleLoginRoute,
    consoleAuthedRoute.addChildren([consoleHomeRoute, consoleWorkspaceRoute]),
  ]),
  workspaceRoute.addChildren([
    overviewRoute,
    contactsRoute,
    contactsCreateRoute,
    contactsEditRoute,
    contactsDetailRoute,
    segmentsRoute,
    segmentsCreateRoute,
    segmentsEditRoute,
    broadcastsRoute,
    broadcastsCreateRoute,
    broadcastsEditRoute,
    broadcastsReportRoute,
    transactionalEmailsRoute,
    templatesRoute,
    templatesCreateRoute,
    templatesEditRoute,
    automationsRoute,
    automationsCreateRoute,
    automationsEditRoute,
    activityRoute,
    settingsRoute,
  ]),
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
