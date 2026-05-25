# Milestone 2 — Grafana-native integration

## Goal

After Milestone 1 the data source is correct and configurable, but Grafana
treats it as a generic backend that only returns frames. Users still cannot
use it for log exploration, annotations, alert rules, dashboard variables, or
ad-hoc filtering — all of which are first-class Grafana features that require
the plugin to opt in via `plugin.json` flags and implement specific handler
contracts.

This milestone makes the plugin a real citizen of the Grafana ecosystem:
declarable in `plugin.json`, queryable from the Explore logs view, usable in
alert rules, and shippable with at least one out-of-the-box dashboard.

## Prerequisites

- Milestone 1 complete (per-instance config, time range, macros, table
  mapping).

## Out of scope

- Monaco editor, syntax highlighting, autocomplete (Milestone 3)
- Visual query builder (Milestone 3)
- Distributed tracing / span ingestion (Milestone 3)

## Requirements

### R2.1 — Logs support

- Set `"logs": true` in `plugin.json`. (Reminder: this requires a Grafana
  restart for users running it locally.)
- Add a `queryType: 'logs'` discriminator on the query model. The frontend
  query editor exposes a "Query type" selector with `timeseries` (default)
  and `logs`.
- For `logs` queries, the backend must emit a frame that Grafana recognizes
  as logs:
  - `Meta.PreferredVisualisation = data.VisTypeLogs`
  - A `time` field (timestamp).
  - A `body` (or `content`) field of type `string` — mapped from DQL
    `content` for `fetch logs`, configurable via an editor field.
  - All remaining columns become **labels** (attached via
    `Field.Labels`) so that they appear in the logs detail panel as
    filterable attributes.
  - Severity column (if present, common DQL names: `loglevel`, `severity`,
    `status`) mapped to `level` field with Grafana's standard enum
    (`critical`, `error`, `warning`, `info`, `debug`, `trace`).
- **Log volume histogram**: implement the `logs sample` API contract — when
  Grafana asks for a supplementary volume query, run a derived DQL that
  buckets the same filter by time and `loglevel`. The query editor exposes
  this as automatic, no user toggle.
- **Derived fields**: in `ConfigEditor`, add a repeatable section to define
  derived fields (regex over `body` → URL/internal link), matching the
  pattern used by other log-capable Grafana data sources.

### R2.2 — Annotations

- Set `"annotations": true` in `plugin.json`.
- Implement the standard annotations contract: a query of type
  `annotations` must return rows with `time` (required), `timeEnd`
  (optional, enables ranges), `title`, `text`, `tags` (string array).
- Editor: reuse the regular `QueryEditor` plus three column-name inputs
  ("Time field", "Title field", "Text field") that the backend uses to map
  arbitrary DQL columns onto the annotation schema.
- Recommended example DQL (shown as placeholder): events from
  `fetch events | filter event.kind == "DAVIS_EVENT"`.

### R2.3 — Alerting

- Set `"alerting": true` in `plugin.json`.
- Verify the backend `QueryData` path works when invoked by the alerting
  scheduler (no panel context, `Headers["FromAlert"]` is set, time range is
  the rule's evaluation interval).
- Macro expansion (R1.4) must work identically in this code path — alerting
  has no `templateSrv` on the frontend, so macros must be applied
  server-side in Go.
- Document one example alert rule in the README (e.g. "host CPU > 80% for
  5 minutes" backed by a `timeseries` DQL query).

### R2.4 — Variable queries

- Implement `metricFindQuery` on the frontend `DataSource` class so that
  dashboard variables of type "Query" can be populated from DQL.
- Editor for variable queries reuses the regular `QueryEditor` but the
  result frame is reduced to a list of `{ text, value }` pairs:
  - If exactly one column is returned, each row becomes
    `{ text: row, value: row }`.
  - If two columns are returned and they are named `text` and `value`
    (case-insensitive), use them; otherwise use the first as both.
- Add a Go helper that, when called from alerting / annotations contexts,
  produces the same `{text, value}` reduction so behavior is consistent
  across surfaces.

### R2.5 — Ad-hoc filters

- Declare ad-hoc filter support: expose a `getTagKeys` and `getTagValues`
  method on the `DataSource` class.
- `getTagKeys` runs a DQL probe (e.g. `smartscapeNodes "HOST" | fieldsKeep
…` or a metadata DQL) and returns the set of dimensions the user can
  filter by. For v1, restrict to a small curated list (host, process,
  service, k8s.namespace) plus any columns observed in the last successful
  query of the current dashboard.
- `getTagValues(key)` runs a `summarize by:{<key>}` query and returns the
  distinct values, capped at 1000.
- Ad-hoc filters are injected into the DQL via a new macro
  `$__adhocFilters` that expands to a `| filter` clause; queries that
  don't reference the macro are unaffected.

### R2.6 — Bundled dashboards

- Ship at least two dashboards in `src/dashboards/` and reference them
  from `plugin.json` `includes`:
  - **Host overview** — CPU, memory, disk, network for hosts selected by a
    template variable.
  - **Log explorer** — log volume histogram + logs panel + severity
    breakdown, filterable by service and host.
- Dashboards must use the `${DS_DYNATRACEGRAIL}` data source variable so
  they import cleanly into any installation.
- A screenshot of each dashboard goes into `src/img/` and the README.

### R2.7 — Provisioning

- Expand `provisioning/` with a complete example:
  - `provisioning/datasources/dynatrace.yaml` with `secureJsonData`.
  - `provisioning/dashboards/dashboards.yaml` referencing the two bundled
    dashboards.
- The `docker compose up` flow should bring up Grafana with the data
  source already configured (token sourced from a `.env` file the user
  creates), so a contributor can go from clone to working panel in one
  command.

### R2.8 — End-to-end tests

- Extend `tests/` with Playwright specs covering:
  - Save & test on a valid / invalid token.
  - Logs query renders the logs visualization.
  - Annotations query returns rows when the time range contains an event.
  - Variable query populates a dropdown.
- Tests run in CI against `GRAFANA_VERSION=latest` and one pinned older
  version (the minimum declared in `plugin.json`).

## Definition of done

- `plugin.json` declares `logs`, `tracing` (stub for Milestone 3),
  `alerting`, `annotations`.
- An alert rule built on a DQL `timeseries` query fires when expected.
- Explore's logs view renders DQL `fetch logs` output with severity colors
  and label filters.
- A dashboard variable populated from `summarize by:{dt.smartscape.host}`
  works in a panel.
- The two bundled dashboards import cleanly into a fresh Grafana and show
  data within 30 seconds of pointing at a live tenant.
- All e2e specs green against the latest Grafana in CI.
