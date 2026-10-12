import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "../common/scalars.tsp";
import "../common/errors.tsp";
import "./resources/identity.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

/** The Operator surface a service calls with a Service credential (ADR 0026). It is served only by the internal listener, never on the public port. */
@service(#{ title: "Sphericon Service API" })
@info(#{ version: "1.0.0" })
@server("http://localhost:3400/service", "Internal listener")
@useAuth(BearerAuth)
namespace Sphericon;
