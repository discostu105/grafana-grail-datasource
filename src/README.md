# Dynatrace Grail (DQL) data source for Grafana

[![License](https://img.shields.io/github/license/discostu105/grafana-grail-datasource)](https://github.com/discostu105/grafana-grail-datasource/blob/main/LICENSE)
[![CI](https://github.com/discostu105/grafana-grail-datasource/actions/workflows/ci.yml/badge.svg)](https://github.com/discostu105/grafana-grail-datasource/actions/workflows/ci.yml)

Query a [Dynatrace](https://www.dynatrace.com/) platform tenant straight from
Grafana using [DQL — the Dynatrace Query Language](https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language).
Build timeseries, table, log, and trace panels, drive alert rules and
dashboard variables, and keep the exact DQL syntax you already use in the
Dynatrace UI.

## Overview

Dynatrace stores observability data in **Grail**, its data lakehouse, and
queries it with **DQL**. This data source proxies DQL straight to a Dynatrace
platform tenant and maps the results onto native Grafana frames, so a query
you wrote in Notebooks renders unchanged in a Grafana panel.

Highlights:

- **One query language, both tools** — paste DQL from Dynatrace Notebooks into
  a Grafana panel and it just runs.
- **Timeseries, tables, logs, and traces** — the backend detects the result
  shape (real Grail `timeframe + interval`, log records, span records) and
  emits the right Grafana frame type, including the Explore logs view and the
  TraceView.
- **Server-side time macros** — `$__timeFilter`, `$__from`, `$__to`,
  `$__interval`, and friends expand on the backend, so alert rules get the same
  substitutions as panels.
- **Grafana-native integrations** — alerting, annotations, template/variable
  queries, dashboard ad-hoc filters, and trace-to-logs / trace-to-metrics
  correlation.
- **Monaco DQL editor** — syntax highlighting, a Format action, and
  autocomplete backed live by Grail's own completion endpoint, plus a visual
  query builder for getting started.
- **Resilient backend** — exponential backoff on 429/5xx (honoring
  `Retry-After`), a per-instance concurrency cap, and Prometheus metrics for
  query latency and outcomes.

## Requirements

- Grafana **>= 12.3.0**.
- A Dynatrace **platform** tenant (`https://<env>.apps.dynatrace.com`) — Grail
  is a platform feature.
- A **platform token** (`dt0s16.*`) with at least:
  - `storage:metrics:read` — timeseries / metrics queries
  - `storage:logs:read` — log queries and the health probe
  - `storage:events:read` — events / problems
  - `storage:spans:read` — trace queries
  - `storage:buckets:read` — required alongside the table scopes above

  Grant only the scopes for the data you plan to query.

## Getting started

1. **Create a platform token** in Dynatrace → **Settings → Access Tokens →
   Platform tokens**, with the scopes listed above.
2. In Grafana go to **Connections → Data sources → Add data source** and search
   for **Dynatrace Grail**.
3. Configure the instance:
   - **Tenant URL** — `https://<env>.apps.dynatrace.com`
   - **API token** — your `dt0s16.*` token (stored encrypted via
     `secureJsonData`)
   - **Query timeout (s)** — default 30; raise for heavy DQL.
   - **Default timeframe** — used when no panel range exists (variable queries,
     alerting probes). A Go duration string, default `1h`.
4. Click **Save & test**. A green check means the token authenticated and a
   minimal probe query (`fetch logs | limit 1`) succeeded.

### Example query

Host CPU bucketed by host, as a timeseries:

```dql
timeseries cpu = avg(dt.host.cpu.usage), by:{dt.smartscape.host}, from:"$__timeFrom", to:"$__timeTo"
```

The `from:`/`to:` parameters scope the metric scan to the panel/alert range at the source. Don't post-filter a timeseries with `$__timeFilter` — that macro is for record queries (`fetch …`).

## Documentation

- **Full README, macros, provisioning, troubleshooting, and DQL primer:**
  <https://github.com/discostu105/grafana-grail-datasource#readme>
- **DQL reference:**
  <https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language>

## Contributing

Issues and pull requests are welcome — see
[CONTRIBUTING.md](https://github.com/discostu105/grafana-grail-datasource/blob/main/CONTRIBUTING.md).
Report bugs and request features on the
[issue tracker](https://github.com/discostu105/grafana-grail-datasource/issues).

## License

[Apache-2.0](https://github.com/discostu105/grafana-grail-datasource/blob/main/LICENSE)

## Disclaimer

This is an unofficial, community-maintained plugin. It is **not** affiliated
with, endorsed by, or sponsored by Dynatrace LLC. "Dynatrace", "Grail", and
"DQL" are trademarks of Dynatrace LLC, used here only to describe what the
plugin connects to.
