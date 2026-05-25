# Changelog

All notable changes to this plugin are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.12.2] - 2026-05-25

### Added

- Visual Query Builder source dropdown is now populated live from
  `fetch dt.system.data_objects | filter type == "table"`. New
  `GET /resources/data-objects` endpoint (cached for an hour) returns
  the real set of fetchable tables on the connected tenant — no more
  hard-coded six-item list. Synthetic entries `metrics (timeseries)` and
  `smartscapeNodes "HOST"` / `"SERVICE"` (#16) stay pinned at the top
  because they use top-level DQL commands, not `fetch`. Falls back to a
  built-in list if the lookup fails.
- New prometheus counter `grafana_dql_data_objects_requests_total{status}`
  for tracking the new resource endpoint.

### Changed

- Pulled in #16: builder no longer suggests deprecated `dt.entity.*`
  sources; emits `smartscapeNodes "HOST"` / `"SERVICE"` verbatim.

## [1.12.1] - 2026-05-25

### Fixed

- Visual Query Builder: produce DQL that actually parses. The 1.12.0
  generator emitted `summarize ... by:{bin(timestamp, …)}` — that's
  SQL-flavoured, not DQL. Time-bucketed series now use `makeTimeseries`
  (events/logs/spans) or `timeseries` (metrics), with `bin()` removed
  from the `by:` clauses.
- Source list aligned with DQL data objects: drop `metric.series` /
  `dt.entity.*` (deprecated), add `bizevents`, `dt.davis.problems`,
  `dt.davis.events`, and `metrics` (which switches the generator to
  the `timeseries` command).
- Operators: rename `matches` → `matchesValue` to match DQL; add hint
  that it's for array fields / wildcard patterns, not substring search
  (use `contains` for that).
- Aggregation: add `percentile` (auto-includes `, 95, rollup: avg` for
  metrics, since percentile/median silently return empty without rollup).
- Filters joined with lowercase `and` (DQL convention).

## [1.12.0] - 2026-05-25

### Added

- Trace status enrichment: `mapStatusCode` now considers `request.is_failed`
  alongside `dt.failure_detection.verdict` / `status.code`, and falls back to
  `endpoint.name` for the trace `operationName` when `span.name` is empty.
- Curated trace-list mode: when the result has a `trace.id` column the
  frontend renames it to `traceID` and stamps an internal `DataLink` that
  re-runs the source query as a span fetch — clickable trace IDs in tables.
- R3.4 trace correlation: `tracesToLogs` / `tracesToMetrics` config sections
  in ConfigEditor stamp `Meta.Custom.tracesToLogs[V2]` / `tracesToMetrics` on
  trace frames so Grafana's TraceView renders Span → Logs / Span → Metrics
  buttons (substitutes `${__span.traceId}` / `${__span.spanId}` at click).
- R3.5 Visual Query Builder: Builder/Code toggle in the QueryEditor with a
  form-driven UI (source, repeatable filters, group-by, aggregation, time
  bucket). A one-way pure-function generator `dqlFromBuilder` produces the
  DQL; switching code → builder confirms before overwriting hand-written DQL.

### Changed

- Lifted shared test selectors into `src/selectors.ts` so Playwright specs and
  components agree on a single source of truth for label/aria text.
- Consolidated the Grail autocomplete proxy call onto the `DataSource` class;
  the Monaco language registration calls `datasource.autocomplete(...)` instead
  of constructing the resource URL itself.
- Replaced the deprecated `HorizontalGroup` in `QueryEditor` with `Stack`.
- Tightened `applyTemplateVariables` / ad-hoc collection to use typed Grafana
  variable models instead of `any` casts.
- Expanded the Go lint set (govet, staticcheck, ineffassign, unused, gocritic,
  misspell, unconvert) and added a drift check that fails the backend tests if
  `pluginVersion` falls out of sync with `package.json`.

## [1.0.0] - Initial release

- Per-instance config (tenant URL + SecureJSON token, query timeout, default
  timeframe).
- Server-side macro expansion: `$__timeFrom/To/from/to/interval/timeFilter`.
- Timeseries + table + logs result shapes; unit + display-name field config
  derived from Grail labels.
- Alerting and annotations support.
- Variable queries (`metricFindQuery`) and dashboard ad-hoc filters with
  Grail-backed key/value discovery.
- Monaco DQL editor with autocomplete proxied through the plugin backend.
- Backend retry with concurrency cap.
