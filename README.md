# grafana-dynatrace-datasource

Grafana data source for [Dynatrace Grail / DQL](https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language).

Query a Dynatrace platform tenant from Grafana panels, alert rules, and dashboard variables — without leaving the DQL syntax you already use in the Dynatrace UI.

## Status

Beta. Tracks the milestones in [`docs/`](docs/). Milestones 1 (correctness & configurability), 2 (Grafana-native integrations) and most of 3 (editor polish, backend resilience) are landed; tracing is the remaining headline gap.

| Capability                                                                    | Status |
| ----------------------------------------------------------------------------- | ------ |
| Per-instance config (URL + SecureJSON token)                                  | ✅     |
| `$__timeFrom/To/from/to/interval/timeFilter` macros (server-side)             | ✅     |
| Template variable interpolation (`$var`, `${var:csv}`)                        | ✅     |
| Timeseries + table result shapes                                              | ✅     |
| Real Grail `timeframe + interval` shape (not just synthetic timestamp arrays) | ✅     |
| Unit + display-name field config from labels                                  | ✅     |
| Alerting + Annotations                                                        | ✅     |
| Variable queries (`metricFindQuery`)                                          | ✅     |
| Logs visualization                                                            | ✅     |
| Ad-hoc filters                                                                | ✅     |
| Monaco DQL editor + Grail-backed autocomplete                                 | ✅     |
| Backend retry + concurrency cap                                               | ✅     |
| Traces                                                                        | 🔜 M3  |

See [`docs/milestone-1-foundations.md`](docs/milestone-1-foundations.md), [`docs/milestone-2-grafana-native.md`](docs/milestone-2-grafana-native.md), and [`docs/milestone-3-editor-traces-polish.md`](docs/milestone-3-editor-traces-polish.md) for the full requirements.

## Configuration

1. **Create a platform token** in Dynatrace → Settings → Access Tokens. Required scopes (minimum): `storage:metrics:read`, `storage:events:read`. Add `storage:logs:read` if you plan to query logs.
2. In Grafana → Connections → Data sources → Add → search **Dynatracegrail**.
3. Fill in:
   - **Tenant URL** — `https://<env>.apps.dynatrace.com`
   - **API token** — your `dt0s16.*` platform token (stored encrypted via `secureJsonData`).
   - **Query timeout (s)** — default 30, raise for heavy DQL.
   - **Default timeframe** — used when no panel range exists (variable queries, alerting probes). Go duration string, default `1h`.
4. Click **Save & test**.

## Provisioning

```yaml
apiVersion: 1
datasources:
  - name: Dynatrace
    type: discostu105-dynatracegrail-datasource
    access: proxy
    jsonData:
      tenantUrl: https://<env>.apps.dynatrace.com
      queryTimeoutSeconds: 30
      defaultTimeframe: 1h
    secureJsonData:
      apiToken: ${DT_TOKEN}
```

## Example queries

**Timeseries** — host CPU bucketed by host:

```dql
timeseries cpu = avg(dt.host.cpu.usage), by:{dt.smartscape.host}
| filter $__timeFilter(timestamp)
```

**Table** — top consumers right now:

```dql
timeseries val = avg(loxone.control.value),
  filter: { control.category == "Energie" AND state.name == "actual" AND unit == "kW" AND control.type == "Meter" },
  by: { control.name }
| fieldsAdd current_kW = arrayLast(val)
| filter isNotNull(current_kW) AND current_kW > 0
| fields control.name, current_kW
| sort current_kW desc
```

**Variable query** — list of host names for a dashboard dropdown:

```dql
smartscapeNodes "HOST"
| fields name
| sort name asc
```

## Macros

| Macro                         | Expands to                                                             |
| ----------------------------- | ---------------------------------------------------------------------- |
| `$__timeFrom` / `$__fromTime` | panel from as RFC3339 string                                           |
| `$__timeTo` / `$__toTime`     | panel to as RFC3339 string                                             |
| `$__from`                     | epoch ms (integer)                                                     |
| `$__to`                       | epoch ms (integer)                                                     |
| `$__interval`                 | DQL duration literal (`1s`, `5s`, …, `1d`) chosen to give ~200 buckets |
| `$__interval_ms`              | ms (integer)                                                           |
| `$__timeFilter(<field>)`      | `<field> >= "<from>" and <field> <= "<to>"`                            |
| `$__timeFilter()`             | same, with `field=timestamp`                                           |

Expansion runs server-side, so alert rules get the same substitutions as panels.

## Bundled dashboards

Four ready-to-use Grafana dashboards translated from the upstream [DynaLox](https://github.com/discostu105/DynaLox) Dynatrace documents live in [`src/dashboards/`](src/dashboards/):

- DynaLox – Energy overview
- DynaLox – Climate & environment
- DynaLox – Lighting controls
- DynaLox – Miniserver health

They reference `${DS_DYNATRACEGRAIL}` so they import cleanly into any installation. Regenerate with [`scripts/convert_dynalox.py`](scripts/convert_dynalox.py) when the upstream changes.

## Build

```bash
npm install
npm run build         # frontend
mage -v buildAll      # backend, all archs
```

## Develop

```bash
npm run dev           # webpack watch
npm run server        # docker compose: grafana + this plugin
```

`provisioning/datasources/datasources.yml` reads `DT_TENANT_URL` and `DT_TOKEN` from the environment for the dev datasource.
