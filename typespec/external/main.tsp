import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";
import "@typespec/versioning";

import "../common/scalars.tsp";
import "../common/pagination.tsp";
import "../common/errors.tsp";
import "../common/versions.tsp";
import "../common/crud.tsp";
import "../common/contact-export.tsp";
import "./resources/contacts.tsp";
import "./resources/segments.tsp";
import "./resources/tags.tsp";
import "./resources/automations.tsp";
import "./resources/broadcasts.tsp";
import "./resources/emails.tsp";
import "./resources/events.tsp";
import "./resources/reference.tsp";
import "./resources/integrations.tsp";
import "./resources/sending-domains.tsp";
import "./resources/consent.tsp";
import "./resources/templates.tsp";
import "./resources/webhooks.tsp";
import "./resources/auth.tsp";
import "./resources/audit.tsp";
import "./mcp.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Versioning;

@service(#{ title: "Sphericon External API" })
@info(#{ version: "1.0.0" })
@server("https://api.getsphericon.app", "Production API")
@server("http://localhost:3000/api", "Local development API")
@useAuth(BearerAuth)
@versioned(Versions)
namespace Sphericon;
