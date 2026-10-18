# Contributing to sphericon

Thanks for helping. Setup, commands and architecture are in [`README.md`](README.md) and
the [documentation](https://docs.getsphericon.app/).

## Contributor License Agreement

Every contributor signs the [Contributor License Agreement](CLA.md) once, before a pull
request can be merged. It lets the Project Owner distribute your contribution under any
license, which sphericon needs as an open-core project (AGPL core, commercial `ee/`).
You keep the copyright in your work.

When you open your first pull request, the CLA assistant comments on it. Reply with the
sentence it asks for, and the `license/cla` check turns green. The signature is tied to
your GitHub account and covers all your later pull requests.

## Enterprise Edition

The `ee/` directory is not open source ([`ee/LICENSE`](ee/LICENSE)). Pull requests that
change `ee/` are accepted only from people with a separate written agreement with the
Project Owner.

## Pull requests

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, ...); release notes are generated from them.
- Run `mise run check` and `mise run test` before you push.
- Generated files are never edited by hand; run `mise run generate` instead.
