import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "../common/limits.tsp";
import "../common/signup-items.tsp";
import "./resources/auth.tsp";
import "./resources/signup-items.tsp";
import "./resources/instance.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon StaffMember API" })
@doc("The core API of the Staff console, served on every Instance (ADR 0063): sign-in with a password and a TOTP, the signed-in Staff member, and the Instance's own settings: its Sign-up items with their Legal documents (ADR 0062), its Telemetry report (ADR 0032), the default Send cap of new Workspaces (ADR 0067) and the Instance audit log (ADR 0070). The Enterprise sections are a contract of their own (staffee), served under the same prefix with the same session cookie.")
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/staff", "Local development staff API")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "STAFF_JWT">)
namespace Sphericon;
