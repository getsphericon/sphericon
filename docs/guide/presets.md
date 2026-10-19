# Presets

A **Preset** describes one provider's inbound callbacks: how a request is proven to come from that
provider, and how its payload becomes **Delivery failures** (a hard bounce or a spam complaint for
one address). Sphericon ships Presets for `generic`, `ses`, `postmark` and `mailgun`. On
a self-hosted Instance you can add your own for a provider the project has no Preset for.

Only the Instance operator supplies Presets: a Workspace never does, and the API accepts none. A
Preset's `transform` runs inside the server process, so it is as trusted as the Instance's
configuration.

::: warning
A bare SMTP server feeds nothing to Suppression. Only a provider that posts callbacks (SES, Postmark,
Mailgun or one you add a Preset for) does. Setup steps for each built-in Preset are in
[Deliverability](/guide/deliverability#bounces-and-complaints); the design is
[ADR 0037](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md).
:::

## Add your own Preset

Put one YAML file per Preset in a directory and point `SPHERICON_PRESETS_DIR` at it (see the
[environment table](/self-hosting#configuration)). Every `*.yaml` and `*.yml` file at the root of the
directory loads at start, beside the built-in ones. A file that does not parse, does not validate
against the schema, has a transform that does not compile, or reuses a built-in name stops the start
with its file and line. Then choose the Preset by name on an Integration.

## Test a Preset before using it

```sh
# run the cases the file carries in its own `tests`
sphericon preset test ./acme-relay.yaml

# built-in Presets run through the same command, by name
sphericon preset test postmark

# run verification and transform on a sample of your own
sphericon preset test ./acme-relay.yaml --payload ./sample.json \
  --header 'X-Signature=sha256=...' --param secret=hunter2
```

Without `--payload` it prints `PASS` or `FAIL` for each case in `tests` and exits non-zero if any
failed. With `--payload` it prints the Delivery failures the sample yields, or the reason it was
refused. `--header name=value` and `--param name=value` repeat; `--now` sets the clock (an RFC 3339
time) for a signature with a timestamp tolerance.

## The file format (version 1)

The format carries a `version` and changes only between Majors, so your files keep working in between.
`sphericon preset schema` prints its JSON Schema (its `$id` is only an identifier, nothing is served
there). Save it next to your file and point your editor at it for validation as you write:

```sh
sphericon preset schema > preset-1.schema.json
```

```yaml
# yaml-language-server: $schema=./preset-1.schema.json
version: 1
name: acme-relay
params: …
verify: …
transform: …
tests: …
```

| Field         | Meaning                                                                                                               |
| ------------- | --------------------------------------------------------------------------------------------------------------------- |
| `version`     | The format version, `1`.                                                                                              |
| `name`        | Unique across the Instance; `[a-z][a-z0-9_-]*`. A custom Preset cannot reuse a built-in name.                         |
| `description` | Optional one line.                                                                                                    |
| `params`      | A JSON Schema (draft 2020-12) of what the Owner enters on the Integration. A secret is a string marked `writeOnly`.   |
| `verify`      | How the request is proven authentic: one kind from the closed set below.                                              |
| `transform`   | A [gojq](https://github.com/itchyny/gojq) program. Input `{body, headers}`; output an array of Delivery failures.     |
| `tests`       | At least one case: `request` (`headers`, `body`), `params`, optional `now`, and the expected `failures` or `refused`. |

### `verify`

Nothing reaches the transform unverified. Header schemes check the raw bytes before any parsing;
body schemes read their signature from the body, so they parse under the body limit, verify, and only
then transform. The body is parsed as JSON whatever its Content-Type; form-encoded bodies are not
supported.

| Kind                | What it checks                                                                                                                         |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `hmac_header`       | HMAC (`algorithm`, `encoding`, optional `prefix`) over a `signed` template (`{body}`, `{header.Name}`), optional timestamp `tolerance` |
| `standard_webhooks` | The three Standard Webhooks headers, five-minute tolerance                                                                             |
| `token`             | A shared token in a `header` (optional `prefix`), or basic auth (`basic: true`)                                                        |
| `hmac_body`         | HMAC over named `signed_fields` of the JSON body, the signature in `signature_field`, optional timestamp field and tolerance           |

Each kind names the `params` property that holds its secret.

### `transform`

A pure function with a fixed output: no environment, no `input`, no modules, and no `debug`, `stderr`, `now` or `input_filename` (a Preset that calls one is refused at load). Each element it returns is
`{id, destination, kind, permanent, sending_domain?}`:

- `id`: the provider's event id; with the destination it dedupes a redelivery.
- `destination`: the address (trimmed and lower-cased by Sphericon).
- `kind`: `bounce` or `complaint`.
- `permanent`: for a bounce, whether it is hard. Only permanent bounces and complaints enter
  Suppression; a soft bounce still counts in the deliverability rates.
- `sending_domain`: optional, the domain the message was sent from.

Sphericon validates what the program returns and never trusts its shape. A run stops at a timeout
(100 ms), and the body, the program (16 KiB) and the number of results (100) are limited. Return an empty
array for an event the Preset does not handle: the provider still gets a 2xx.
