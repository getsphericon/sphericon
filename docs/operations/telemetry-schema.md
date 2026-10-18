---
aside: false
outline: false
title: Telemetry report schema
---

<script setup lang="ts">
import spec from '../../openapi/telemetry.openapi.json'
</script>

# Telemetry report schema

The exact body of the anonymous Telemetry report an Instance sends once a day. It carries no
name, slug, domain, address, host, error text, licence key or exact number. Counts are orders of
magnitude, never exact figures. The schema below is the contract: every report an Instance builds
is checked against it.

The raw document is
[`openapi/telemetry.openapi.json`](https://github.com/getsphericon/sphericon/blob/main/openapi/telemetry.openapi.json)
in the repository.

<OAOperation :spec="spec" operationId="TelemetryReports_submit" />
