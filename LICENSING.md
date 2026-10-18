# Licensing

sphericon is **open-core**: the core is free and open source, and a small set of
enterprise features is commercial.

## The split

- **Open core — GNU AGPL-3.0.** Everything in this repository is licensed under the
  GNU Affero General Public License v3.0 (see [`LICENSE`](./LICENSE)) **except** the
  files noted below. You can self-host, modify, and run the core freely; if you offer
  it as a network service, the AGPL requires you to publish your source.

- **Enterprise Edition — commercial.** Everything under [`ee/`](./ee/) (and any file
  carrying an explicit "sphericon Enterprise Edition License" header) is **source-available
  but not open source**, governed by [`ee/LICENSE`](./ee/LICENSE). It may be used in
  production only with a valid sphericon Enterprise subscription (license key).

## How Enterprise ships

Enterprise code is compiled into the **same single binary** as the core; its features
remain locked until a valid license key is present at runtime. We distribute one
artifact for everyone — there is no separate enterprise build.

## Third-party code in the repository

The `migrations/*_river_*.sql` files carry the SQL of [river](https://github.com/riverqueue/river)
verbatim, written by `mise run db:river-migration`. river is licensed under the Mozilla Public
License 2.0, which applies to those files; the rest of the repository keeps its own license.

## Questions

For commercial licensing or an Enterprise subscription, contact the sphericon team.
