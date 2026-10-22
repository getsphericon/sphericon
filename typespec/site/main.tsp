import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/pagination.tsp";
import "../common/errors.tsp";
import "../common/crud.tsp";
import "../common/contact-export.tsp";
import "../common/lint.tsp";
import "../common/scopes.tsp";
import "./resources/consent.tsp";
import "./resources/contacts.tsp";
import "./resources/custom-fields.tsp";
import "./resources/segments.tsp";
import "./resources/tags.tsp";
import "./resources/auth.tsp";
import "./resources/workspaces.tsp";
import "./resources/memberships.tsp";
import "./resources/invitations.tsp";
import "./resources/me.tsp";
import "./resources/second-factor.tsp";
import "./resources/identities.tsp";
import "./resources/passkeys.tsp";
import "./resources/passkey-login.tsp";
import "./resources/events.tsp";
import "./resources/tokens.tsp";
import "./resources/integrations.tsp";
import "./resources/link-clicks.tsp";
import "./resources/broadcasts.tsp";
import "./resources/templates.tsp";
import "./resources/brand-kit.tsp";
import "./resources/join-domains.tsp";
import "./resources/automations.tsp";
import "./resources/enrollments.tsp";
import "./resources/rules.tsp";
import "./resources/webhooks.tsp";
import "./resources/sending-domains.tsp";
import "./resources/analytics.tsp";
import "./resources/setup-checklist.tsp";
import "./resources/suppressions.tsp";
import "./resources/transactional-emails.tsp";
import "./resources/oauth.tsp";
import "./resources/audit.tsp";
import "./resources/instance.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon Site API" })
@doc("The API of the Sphericon web app, authenticated by the session cookie.")
@info(#{ version: "1.0.0" })
@server("http://localhost:3000", "Local development site API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "JWT">)
namespace Sphericon;
