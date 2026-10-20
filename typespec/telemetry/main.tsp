import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

import "./resources/report.tsp";

using TypeSpec.OpenAPI;
using TypeSpec.Http;

/**
 * The anonymous Telemetry report an Instance sends once a day (ADR 0032). The receiver is a
 * separate service outside this repository; this document is the contract between the two.
 */
@service(#{ title: "Sphericon Telemetry API" })
@info(#{ version: "1.0.0" })
@server("https://telemetry.getsphericon.com", "Telemetry receiver")
namespace Sphericon;
