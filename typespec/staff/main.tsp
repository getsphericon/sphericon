import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "../common/pagination.tsp";
import "./resources/auth.tsp";
import "./resources/workspaces.tsp";
import "./resources/audit.tsp";
import "./resources/signup-survey.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon StaffMember API" })
@doc("The API of the platform Staff console (Enterprise): sign-in with a password and a TOTP, and the cross-workspace views and actions of a Staff member.")
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/staff", "Local development staff API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "STAFF_JWT">)
namespace Sphericon;
