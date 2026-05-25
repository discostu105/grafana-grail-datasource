# Milestone 3 — Editor, traces, and polish

## Goal

After Milestones 1 and 2 the plugin is correct, configurable, and integrated
with Grafana's logs / alerting / annotations / variables. What still separates
it from a mature, public-catalog-quality data source is **authoring experience**
and **trace data**:

- The query editor is a plain textarea — no syntax highlighting, no
  autocomplete, no formatting, no error markers, no run shortcut.
- DQL has a richer surface (`fetch spans`, `smartscapeNodes`) than the
  current frame mapper exposes; in particular, distributed traces are
  not surfaced as Grafana traces.
- Operational polish (request tracing, retry, rate limiting, branding,
  catalog readiness) is missing.

This milestone takes the plugin from "fully functional internal tool" to
"shippable in the public plugin catalog".

## Prerequisites

- Milestones 1 and 2 complete.

## Out of scope

- Profiling support (not currently a DQL primary).
- Built-in DQL formatter / linter as a public API (we ship a basic
  implementation but do not promise stability).

## Requirements

### R3.1 — Monaco-based DQL editor

- Replace the `<textarea>` in `QueryEditor.tsx` with a Monaco editor (use
  `@grafana/ui`'s `CodeEditor` so we inherit theme + sizing).
- Define a DQL language contribution:
  - **Tokenizer** covering DQL verbs (`fetch`, `filter`, `summarize`,
    `fields…`, `parse`, `join`, `lookup`, `timeseries`, `makeTimeseries`,
    `sort`, `limit`), operators, string/number/duration literals, and
    comments.
  - Bracket matching, auto-closing pairs for `(`, `[`, `{`, `"`, `'`.
  - A monarch language id `dql` registered globally so other contexts
    (alert rule editor) get the same highlighting.
- Editor toolbar with: format selector (timeseries / table / logs / trace),
  "Run query" button, "Format DQL" button (best-effort indenter), and a
  link to the DQL reference docs.
- Keyboard: `Ctrl/Cmd + Enter` runs the query; `Shift + Alt + F` formats.

### R3.2 — Autocomplete and inline schema discovery

- Register a Monaco completion provider that suggests:
  - DQL verbs / keywords (static list).
  - Field names observed in the **last successful response** for the same
    refId (cached client-side per editor instance).
  - Dimensions and metrics from a backend "list keys" call. Add a Go
    resource handler `GET /resources/keys?prefix=…` that runs a DQL probe
    (`fetch metric.series | summarize by:{metric.key} | limit 200` or
    similar) and returns matches.
  - Template variables (`$varname`) from `templateSrv.getVariables()`.
- Trigger characters: `.`, `:`, `,`, space after a pipe.
- Each completion item carries a documentation snippet (kept in a static
  `dqlDocs.ts` file) so users see "what this verb does" inline.

### R3.3 — Traces support

- Set `"tracing": true` in `plugin.json` (we'll already have stubbed it in
  M2; this milestone makes it real).
- Add `queryType: 'traces'` to the query model. Two sub-modes:
  - **Trace list** — `fetch spans | summarize by:{trace.id}` style; renders
    as a table of trace IDs with duration, service, root operation. Each
    row links to a trace detail.
  - **Trace detail** — `fetch spans | filter trace.id == "<id>"`; mapped
    to a Grafana traces frame (`Meta.PreferredVisualisation =
data.VisTypeTrace`) with the OpenTelemetry-compatible field set
    (`traceID`, `spanID`, `parentSpanID`, `operationName`, `serviceName`,
    `startTime`, `duration`, `tags`, `logs`).
- Backend mapper `pkg/plugin/traces.go` produces the frame; unit tests
  cover a small captured span response.

### R3.4 — Trace-to-logs / trace-to-metrics links

- In `ConfigEditor`, add the standard "Trace to logs" and "Trace to
  metrics" sections (mirror the contract used by other Grafana trace
  data sources):
  - Target data source picker.
  - Tag mapping (span tag → log/metric label).
  - Query template (DQL with `${__span.traceId}` placeholder).
- Persist these in `jsonData.tracesToLogs` and `jsonData.tracesToMetrics`;
  the frontend passes them to the traces view via frame `Meta.Custom`.

### R3.5 — Visual query builder (basic)

- Behind a "Builder / Code" toggle (matching the pattern other SQL-ish
  data sources use), expose a form-based builder for the most common
  shape:
  - Data source: dropdown (`metrics`, `logs`, `events`, `spans`,
    `smartscapeNodes "HOST"` / `"SERVICE"`).
  - Filters: repeatable `field` `operator` `value` rows.
  - Group by: multi-select of dimensions discovered via R3.2.
  - Aggregation: `count` / `avg` / `sum` / `min` / `max` plus the field
    they operate on.
  - Time bucketing: dropdown (`auto`, `1m`, `5m`, `1h`).
- The builder is a one-way generator: each change rewrites the DQL in the
  Monaco editor. Switching back to code mode keeps user edits; switching
  to builder mode after manual edits warns "this will overwrite your
  query".
- Builder is optional UX; advanced users live in code mode. The builder
  does not need to handle every DQL feature — explicitly link to code
  mode for `join`, `lookup`, `parse`, etc.

### R3.6 — Result field configuration

- Map richer Grail metadata onto Grafana field config:
  - Decimal places from the column's declared precision.
  - Min / max from metadata when present (drives gauges).
  - Display name templating from group-by dimensions (e.g.
    `{{dt.smartscape.host}}`).
- Add a per-query "Series naming" input (templated string) that overrides
  the default legend, mirroring the experience of mature data sources.

### R3.7 — Resilience and observability

- Wrap Grail calls with:
  - Retry with exponential backoff on `429` and `5xx`, max 3 attempts,
    honoring `Retry-After`.
  - Per-instance concurrency limit (configurable, default 8) to avoid
    saturating the Grail query budget.
  - Context cancellation: if the Grafana request context is cancelled,
    propagate to the Grail poll and surface a "query cancelled" notice
    instead of an opaque error.
- Add structured logging at info level for each query (refId, DQL length,
  duration, row count) and at debug for the raw shape (existing behavior).
- Export Prometheus metrics from the backend via the Grafana plugin SDK:
  request count, request duration histogram, error count by status code.

### R3.8 — Catalog readiness

- Real branding: SVG logo (not the scaffold default), 1280×640 cover
  image, three screenshots in `src/img/`.
- `plugin.json` polished:
  - `info.description` — one sentence pitched at a Grafana admin.
  - `info.keywords` — actual keywords (`dql`, `observability`,
    `metrics`, `logs`, `traces`).
  - `info.links` — homepage, docs, issue tracker.
  - `info.screenshots` — populated.
- README expanded with a feature matrix, a DQL primer for users coming
  from PromQL / SQL, and a troubleshooting section.
- `CHANGELOG.md` is current and follows Keep-a-Changelog format.
- Plugin signs and passes `npx @grafana/plugin-validator` cleanly.

### R3.9 — Test coverage

- Backend ≥ 80% line coverage on `pkg/plugin/` (frames, traces, macros).
- Frontend unit tests for: the Monaco language contribution (tokenizer
  golden tests), the completion provider, the builder→DQL generator.
- E2E specs added for: traces view, trace-to-logs link, autocomplete in
  the editor, builder mode round-trip.

## Definition of done

- A new user, with no prior DQL knowledge, can type a query in the
  builder, switch to code mode to refine it, and ship it to a dashboard
  in under 5 minutes.
- A trace ID followed from a logs panel opens a full waterfall in the
  traces view.
- `npx @grafana/plugin-validator` reports zero errors and zero warnings.
- The plugin is submitted to the Grafana catalog and accepted on first
  review (no missing-asset or missing-doc rejections).
