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

@service(#{ title: "Sphericon Operator API" })
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/operator", "Local development operator API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "OPERATOR_JWT">)
namespace Sphericon;
