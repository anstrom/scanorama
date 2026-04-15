# CLI/API Parity — High-Priority Gaps (issue #711)

## Scope

Four gaps from the "High priority" tier in issue #711, addressed in a single PR:

1. `groups` subcommand — full CRUD + member management
2. `settings` subcommand — view and update runtime config
3. `smart-scan` subcommand — suggestions, profile-recommendations, stage, trigger
4. `schedule show/list` — add `--output json`; surface next-run from server API

Medium and lower priority items are deferred to a follow-up issue.

---

## Architecture

All four additions follow the same API-client pattern already used by `apikeys.go`:
- `mustCreateAPIClient()` / `WithAPIClient()` for auth
- `--output json` → `json.MarshalIndent` to stdout; default → tablewriter table
- Error handling via `handleAPIError()`
- One new `.go` file per subcommand; tests in matching `_test.go`

---

## groups subcommand (`cmd/cli/groups.go`)

```
groups list               [--output json]
groups show <id>          [--output json]
groups create <name>      [--description TEXT] [--color HEX] [--output json]
groups update <id>        [--name NAME] [--description TEXT] [--color HEX]
groups delete <id>
groups members <id>       [--output json]
groups add-member <id>    --hosts <h1,h2,...>
groups remove-member <id> --hosts <h1,h2,...>
```

Endpoints: `GET/POST /groups`, `GET/PUT/DELETE /groups/{id}`,
`GET/POST/DELETE /groups/{id}/hosts`.

Response fields used for display:
- Group: `id`, `name`, `description`, `color`, `created_at`, `member_count`
- Member: `id`, `ip_address`, `hostname`, `status`, `last_seen`

---

## settings subcommand (`cmd/cli/settings.go`)

```
settings get              [--output json]
settings update           --key KEY --value VALUE
```

Endpoints: `GET /admin/settings`, `PUT /admin/settings`.

`settings update` sends a partial JSON body `{ "<key>": "<value>" }`. The API
accepts a partial object and merges; CLI maps `--key`/`--value` to a
`map[string]string` for the request body.

---

## smart-scan subcommand (`cmd/cli/smart_scan.go`)

```
smart-scan suggestions                [--output json]
smart-scan profile-recommendations    [--output json]
smart-scan stage <host-id>            [--output json]
smart-scan trigger <host-id>
smart-scan trigger-batch              [--stage STAGE] [--limit N]
```

Endpoints: `GET /smart-scan/suggestions`, `GET /smart-scan/profile-recommendations`,
`GET /smart-scan/hosts/{id}/stage`, `POST /smart-scan/hosts/{id}/trigger`,
`POST /smart-scan/trigger-batch`.

For `trigger-batch`, `--stage` maps to the `BatchFilter.Stage` field;
`--limit` maps to `BatchFilter.MaxHosts`.

---

## schedule changes (`cmd/cli/schedule.go`)

- Add `scheduleOutput string` flag to `list` and `show` commands (`--output json`, default `table`).
- `schedule show` with `--output json`: after loading the job from DB, additionally
  call `GET /schedules/{id}/next-run` via the API client (best-effort; skip if API
  unavailable) and include the server-computed next-run in JSON output.
- Table display is unchanged.

---

## Testing

Each new file gets a `_test.go` covering:
- Command registration and help text (Cobra structure)
- Happy-path API mock (success response → expected table/JSON output)
- Error path (API returns 404 or 500 → error message to stderr, non-zero exit)
- `--output json` → valid JSON with expected top-level keys

Tests use `httptest.NewServer` to serve mock API responses, setting
`SCANORAMA_API_KEY` and pointing `baseURL` at the test server.
