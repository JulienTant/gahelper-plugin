# GA Helper (dev-only)

> [!WARNING]
> **This is not an official Mattermost plugin.** It is a personal, throwaway,
> dev-only tool with no support, no tests, and no CI coverage of its actual
> behavior. It is **not** associated with any Mattermost ticket or PR, and it
> is not intended to be installed on any server other than a local dev
> instance. Do not deploy this anywhere else.

[![Build Status](https://github.com/JulienTant/gahelper-plugin/actions/workflows/ci.yml/badge.svg)](https://github.com/JulienTant/gahelper-plugin/actions/workflows/ci.yml)

## What this is

This plugin exists purely so a human can manually create `access_control`/`template`
property fields with arbitrary `attrs` (especially `source_plugin_id` + `protected`,
`ldap`, `saml`) to visually test the **Source** column of the Global Attributes /
"Manage Attributes" admin console page in the `mattermost` repo.

It writes directly to Postgres, bypassing the property system's app-layer validation
entirely, on purpose — that's the only way to exercise this particular UI branch
locally. This pattern should never be copied into production plugin code.

See [`CLAUDE.md`](CLAUDE.md) for the full explanation of what it does, why, and how.

## Building and running locally

Requires Go modules; be sure this project lives outside `$GOPATH`.

Build the plugin:
```
make
```

This produces a plugin bundle for upload to your local Mattermost server:
```
dist/com.mattermost.gahelper-*.tar.gz
```

### Deploying to a local dev server

Enable plugin uploads first via `config.json` or the System Console:
```json
    "PluginSettings" : {
        ...
        "EnableUploads" : true
    }
```

If your server has [local mode](https://docs.mattermost.com/administration/mmctl-cli-tool.html#local-mode) enabled:
```
make deploy
```

Or authenticate with credentials / a personal access token:
```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=<your-token>
make deploy
```

If developing the webapp side, watch for changes and deploy automatically:
```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=<your-token>
make watch
```
