import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "./resources/errors.tsp";
import "./resources/identify.tsp";
import "./resources/events.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

@service(#{ title: "Sphericon Collect API" })
@doc("Event ingestion for customer sites and apps, authenticated by a collect key.")
@info(#{ version: "1.0.0" })
@server("http://localhost:3000/collect", "Local development collect API")
@useAuth(ApiKeyAuth<ApiKeyLocation.header, "x-collect-key">)
namespace Sphericon;
