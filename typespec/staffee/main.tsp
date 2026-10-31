import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "../common/pagination.tsp";
import "../common/limits.tsp";
import "./resources/workspaces.tsp";
import "./resources/audit.tsp";
import "./resources/signup-survey.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon Staff Enterprise API" })
@doc("The Enterprise sections of the Staff console (license feature `staff`, ADR 0026, ADR 0063): the cross-workspace views and actions of a Staff member. Served under /staff beside the core Staff API, with its session cookie; without the feature every route answers 404.")
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/staff", "Local development staff API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "STAFF_JWT">)
namespace Sphericon;
