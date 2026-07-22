# gahelper — throwaway local-dev plugin

**This is NOT production code and is NOT part of any Mattermost ticket/PR.** It exists purely so a
human can manually create `access_control`/`template` property fields with arbitrary `attrs`
(especially `source_plugin_id` + `protected`, `ldap`, `saml`) to visually test the **Source**
column of the Global Attributes / "Manage Attributes" admin console page
(`mattermost` repo, ticket MM-69846: `webapp/channels/src/components/admin_console/global_attributes/`).

That plugin-source branch can't be exercised any other way: the admin REST API strips/rejects
`source_plugin_id`/`protected` from non-plugin callers, and even a *real* plugin using the
official plugin API only ever gets its own true plugin ID auto-stamped — never an arbitrary one.
So this tool writes directly to Postgres, bypassing the property system's app-layer validation
entirely, on purpose.

Sibling to the `mattermost` monorepo, deliberately kept **outside** it
(`/Users/julientant/projects/mattermost/gahelper-plugin`, not nested inside `mattermost/`) so it
never shows up in `git status` for any ticket's PR.

## Origin

Cloned from https://github.com/mattermost/mattermost-plugin-starter-template and renamed
throughout (plugin id `com.mattermost.gahelper`, module `github.com/julientant/gahelper-plugin`),
now hosted at https://github.com/JulienTant/gahelper-plugin.

## What it does

Two slash commands, registered in `server/command/command.go`:

- **`/gahelper create-field`** — opens an interactive dialog (`createFieldDialog()`) with fields:
  - `name` — `PropertyFields.Name`, a slug-like identifier
  - `display_name` — optional, falls back to `name`
  - `type` — select, intentionally restricted to `text` / `select` / `multiselect` / `rank` only
    (the four types the Global Attributes Type column has real label mappings for — `date`/
    `user`/`multiuser` are deliberately left out of this dialog since they're only useful for
    testing the Type column's unmapped-type fallback, not this tool's purpose)
  - `options` — textarea, one option per line, only meaningful for select/multiselect/rank
  - `source` — select: `none` (falls through to "Managed here"), `ldap`, `saml`, `plugin`.
    Only ONE of these is ever set on a given field, matching the real webapp resolution order
    (`getSourceKind` in `global_attributes_table.tsx`): plugin+protected → ldap → saml → managed.
  - `ldap_attribute` / `saml_attribute` — optional follow-up text fields, only used when `source`
    matches.

  Submission posts to `server/api.go`'s `/dialog/create-field` route, which calls
  `gadb.CreateField` to INSERT the row directly.

- **`/gahelper delete-field <name>`** — soft-deletes (`DeleteAt = now`) the named
  `access_control`/`template` field via `gadb.DeleteFieldByName`. No confirmation prompt — this is
  a throwaway tool, not a production delete flow.

## How the DB access works

`server/gadb/gadb.go` holds all the direct-SQL logic. It gets a real `*sql.DB` via
`pluginapi.Client.Store.GetMasterDB()` — the server's actual connection pool through the plugin
RPC driver — rather than hand-rolling a separate connection from a DSN string. This mirrors the
real Playbooks plugin's pattern (see `mattermost-plugin-playbooks/server/sqlstore/` if you need a
reference for other direct-DB plugin patterns).

Schema targeted (Postgres, from `mattermost/server/channels/db/migrations/postgres/`):
`PropertyGroups(ID, Name)` and `PropertyFields(ID, GroupID, Name, Type, Attrs jsonb, TargetID,
TargetType, CreateAt, UpdateAt, DeleteAt, ObjectType, Protected, PermissionField,
PermissionValues, PermissionOptions, LinkedFieldID)`. `gadb.EnsureAccessControlGroup` creates the
`access_control` PropertyGroups row if it's missing (resolving the group name to its real ID,
since `PropertyFields.GroupID` is a reference to that ID, not the literal string
`"access_control"`).

`GAHelperPluginID = "com.mattermost.gahelper"` is what gets stamped into `attrs.source_plugin_id`
when demoing the "plugin" source branch — it doesn't correspond to any other real installed
plugin.

## Build / deploy

Standard plugin-starter-template Makefile targets:

```bash
make dist    # go build/vet + webapp build + bundle into dist/com.mattermost.gahelper-*.tar.gz
make deploy  # dist + upload to a running local server (needs deploy env vars, see Makefile)
```

Actually used to get it onto the local dev server (`http://localhost:8065`, sysadmin /
`Sys@dmin-sample1`):

```bash
cd /Users/julientant/projects/mattermost/mattermost/server
./bin/mmctl --local plugin add /Users/julientant/projects/mattermost/gahelper-plugin/dist/com.mattermost.gahelper-*.tar.gz --force
./bin/mmctl --local plugin enable com.mattermost.gahelper
./bin/mmctl --local plugin list   # confirm it shows under "Listing enabled plugins"
```

(`mmctl` isn't on `$PATH` in this environment — run it from the built binary in the `mattermost`
server repo, `server/bin/mmctl`.)

## Rules for working in this repo

- Never move any of this logic into the `mattermost` repo or a ticket's PR — it's scratch tooling
  for one person's local dev environment.
- Direct-DB-write is fine here specifically because this is throwaway; never copy this pattern
  into production plugin code.
- Don't add tests, don't polish error handling beyond "doesn't crash" — this doesn't need to be
  correct for inputs a human won't actually type.
- If the Global Attributes ticket's Source-column logic or schema changes, this plugin will
  silently drift out of sync — it's not covered by any CI, so re-check `gadb.go`'s attrs shape
  against the current `global_attributes_table.tsx`/migrations by hand if things stop working.
