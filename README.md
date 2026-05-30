# Grail (DQL) data source for Grafana

Grafana data source for [Dynatrace Grail / DQL](https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language).

Query a Dynatrace platform tenant from Grafana panels, alert rules, and dashboard variables — without leaving the DQL syntax you already use in the Dynatrace UI.

## Status

Beta. Tracks the milestones in [`docs/`](docs/). Milestones 1 (correctness &
configurability), 2 (Grafana-native integrations) and 3 (editor polish, traces,
backend resilience) are landed.

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
| Monaco DQL editor (highlighting, Format, Grail-backed autocomplete)           | ✅     |
| Visual query builder                                                          | ✅     |
| Backend retry + concurrency cap + Prometheus metrics                          | ✅     |
| Traces (trace list + detail, trace-to-logs / trace-to-metrics)                | ✅     |

**Known limitations:** annotations work via Grafana's standard "use query
result" path (no dedicated time/title/text column editor yet). See the
milestone docs for the per-requirement status:
[`docs/milestone-1-foundations.md`](docs/milestone-1-foundations.md),
[`docs/milestone-2-grafana-native.md`](docs/milestone-2-grafana-native.md),
[`docs/milestone-3-editor-traces-polish.md`](docs/milestone-3-editor-traces-polish.md).

## Configuration

1. **Create a platform token** (`dt0s16.*`) in Dynatrace → **Settings → Access Tokens → Platform tokens**. Grant the scopes for the data you intend to query:
   - `storage:metrics:read` — timeseries / metrics
   - `storage:logs:read` — logs (also used by the health probe)
   - `storage:events:read` — events / problems
   - `storage:spans:read` — traces
   - `storage:buckets:read` — required alongside the table scopes above
2. In Grafana → Connections → Data sources → Add → search **Grail (DQL)**.
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
    type: discostu105-grail-datasource
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

**Table** — top hosts by current CPU:

```dql
timeseries cpu = avg(dt.host.cpu.usage), by:{dt.smartscape.host}
| fieldsAdd current = arrayLast(cpu)
| filter isNotNull(current)
| fields dt.smartscape.host, current
| sort current desc
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

## Coming from PromQL or SQL?

DQL is a pipeline language: a source command, then `|`-separated transforms.
A few rough analogies to get oriented (DQL is not PromQL or SQL — see the
[DQL reference](https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language)
for the real semantics):

| You want…                 | PromQL / SQL                   | DQL                                              |
| ------------------------- | ------------------------------ | ------------------------------------------------ |
| A metric over time        | `avg(rate(http_requests[5m]))` | `timeseries r = avg(dt.service.request.count)`   |
| Group by a dimension      | `... by (host)`                | `timeseries x = avg(m), by:{dt.smartscape.host}` |
| Filter rows               | `WHERE status = 500`           | `\| filter status == 500`                        |
| Select columns            | `SELECT a, b`                  | `\| fields a, b`                                 |
| Add a computed column     | `SELECT a+b AS c`              | `\| fieldsAdd c = a + b`                         |
| Sort / limit              | `ORDER BY x DESC LIMIT 10`     | `\| sort x desc \| limit 10`                     |
| Aggregate non-metric data | `GROUP BY host`                | `\| summarize count(), by:{host}`                |

Key gotchas: equality is `==` (not `=`), boolean operators are lowercase
`and` / `or`, and time bucketing on logs/events/spans uses `makeTimeseries`
(use `timeseries` only for the metrics store).

## Alerting

Alert rules run the same DQL path as panels, with the same macro expansion. Use
a query that returns a numeric timeseries and add a Grafana threshold/reduce
expression on top. Example — alert when average host CPU exceeds 90%:

```dql
timeseries cpu = avg(dt.host.cpu.usage), by:{dt.smartscape.host}
| filter $__timeFilter(timestamp)
```

In the alert rule, add a **Reduce** (Last) and a **Threshold** (`IS ABOVE 90`)
expression on the `cpu` series. Because macros expand server-side, the rule
evaluates over the alert's own time window — no panel range required.

## Annotations

Set the dashboard annotation query's data source to this plugin and write DQL
that returns a time column plus optional `text` / `title` columns, e.g.:

```dql
fetch events
| filter event.kind == "DEPLOYMENT_EVENT"
| fields timestamp, title = event.name, text = event.description
```

Grafana's standard "use query result" annotation path renders one marker per
row. (A dedicated time/title/text column picker is on the roadmap; today the
column names must match Grafana's expectations as shown above.)

## Troubleshooting

| Symptom                                        | Likely cause / fix                                                                                                                           |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| **Save & test:** "Authentication rejected"     | Token is invalid or expired. Create a fresh `dt0s16.*` platform token.                                                                       |
| **Save & test:** "Token lacks required scopes" | Add the missing `storage:*:read` scope (see [Configuration](#configuration)). The probe needs `storage:logs:read`.                           |
| **Save & test:** "Cannot reach …" / TLS error  | Tenant URL is wrong or unreachable. Use the `https://<env>.apps.dynatrace.com` (platform) host, not the classic `*.live.dynatrace.com` host. |
| Query returns empty but no error               | Your time range has no data, or a `filter` is too strict. Check the panel's **Inspect → Query** for the expanded DQL and any Grail notices.  |
| "dqlQuery is empty"                            | The panel has no DQL text. Enter a query or switch to the visual builder.                                                                    |
| Timeouts on heavy queries                      | Raise **Query timeout (s)** in the data source config and narrow the DQL (`limit`, tighter filters, longer bucket size).                     |
| Percentile/median returns empty for metrics    | DQL needs an explicit `rollup` for those — the visual builder adds `, 95, rollup: avg` automatically; do the same in hand-written DQL.       |

Grail sampling/scan-limit notices are surfaced as panel notices — open
**Inspect → Query** to see them.

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
