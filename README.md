# grafana-dql-datasource

A Grafana data source plugin for [Dynatrace Grail / DQL](https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language).

## Status

v1 — minimum viable:

- Per-instance config: tenant URL (JSONData) + platform token (SecureJSON)
- Plain textarea query editor (no syntax highlighting yet)
- Time range: panel's `From`/`To` is passed as DQL `DefaultTimeframeStart/End`; `$__timeFrom` / `$__timeTo` macros are substituted in the DQL string
- Result mapping: timeseries (one frame per series) and table (single tabular frame). Other shapes fall back to table.

## Configuration

In the data source settings page:

- **Tenant URL** — e.g. `https://abc.apps.dynatrace.com`
- **API token** — a platform token (`dt0s16.…`) with `storage:metrics:read` / `storage:events:read` etc.

Click **Save & test** — it runs `data record(x = 1)` against the tenant.

## Build

```bash
npm install
npm run build          # frontend
mage -v buildAll       # backend (all archs into dist/)
```

## Develop

```bash
npm run dev            # webpack watch
npm run server         # docker compose: grafana + this plugin
```

Provisioning in `provisioning/datasources/datasources.yml` reads `DT_TENANT_URL` and `DT_TOKEN` from the environment for the dev datasource.

## Repo layout

- `pkg/plugin/datasource.go` — backend entry: settings → client, QueryData, CheckHealth, macro substitution
- `pkg/plugin/frames.go` — DQL records → Grafana data frames (timeseries / table)
- `pkg/dynatrace/client.go` — thin wrapper over `dtctl` SDK
- `src/components/ConfigEditor.tsx` — tenant URL + API token form
- `src/components/QueryEditor.tsx` — DQL textarea
