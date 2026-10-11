# Sphericon Cloud

Sphericon Cloud is the hosted edition of sphericon, run by the people who build it. It is the same
product you can [self-host](/self-hosting), so everything in this guide applies to both.

## When the hosted edition fits

- You want to start sending without provisioning PostgreSQL, backups or upgrades.
- Nobody on your team wants to be on call for [monitoring](/operations/monitoring) and the
  [runbook](/operations/runbook).
- You would rather keep the option to move later: the data model, the [API](/guide/api) and the
  [tracker](/guide/tracking) are identical, so leaving for your own server is a migration, not a rewrite.

## What changes compared to self-hosting

| Topic        | Self-hosted                                         | Sphericon Cloud        |
| ------------ | --------------------------------------------------- | ---------------------- |
| Running it   | You run the binary or image and PostgreSQL          | Run for you            |
| Upgrades     | You apply them ([Upgrading](/operations/upgrading)) | Applied for you        |
| Backups      | You schedule and test them                          | Handled for you        |
| API host     | Your own origin                                     | `api.getsphericon.app` |
| Tracker host | Your own origin                                     | `t.getsphericon.app`   |
| MCP host     | Your own origin ([MCP for agents](/guide/mcp))      | `mcp.getsphericon.app` |

[Create a workspace on Sphericon Cloud](https://getsphericon.app) or keep reading to run it yourself.
