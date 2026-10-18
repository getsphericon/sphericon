import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "../common/pagination.tsp";
import "./resources/auth.tsp";
import "./resources/workspaces.tsp";
import "./resources/audit.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon StaffMember API" })
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/staff", "Local development staff API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "STAFF_JWT">)
namespace Sphericon;
