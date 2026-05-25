# Milestone 1 — Foundations

## Goal

Make the data source usable by more than one developer on one laptop. Today the
plugin reads credentials from process environment variables, ignores the
dashboard time picker, and only renders one shape of DQL result. After this
milestone the plugin should be installable from a Grafana provisioning file,
configurable per instance through the UI with encrypted secrets, honor the
panel's time range, and produce correct frames for the two query shapes users
actually run (`timeseries` and `fetch`).

This milestone is about **correctness and basic usability**. No new UI
frameworks, no editor rewrite, no new Grafana capabilities — just turning the
prototype into something safe to point at a production tenant.

## Out of scope

- Logs / traces / annotations / alerting (Milestone 2)
- Monaco editor, autocomplete, visual builder (Milestone 3)
- Bundled dashboards (Milestone 2)

## Requirements

### R1.1 — Per-instance configuration with encrypted secrets

- Replace `ConfigEditor.tsx` with a real form containing:
  - **Tenant URL** (text input, validated as `https://*.apps.dynatrace.com` or
    `https://*.live.dynatrace.com`; stored in `jsonData`).
  - **Platform token** (`SecretInput` from `@grafana/ui`; stored in
    `secureJsonData.token`, with reset behavior).
  - **Query timeout** (numeric, seconds, default 30, stored in `jsonData`).
  - **Default query timeframe** override (optional; only used when DQL omits
    `from:`/`to:` and Grafana provides no range — e.g. variable queries).
- `DqlDataSourceOptions` gains `tenantUrl: string`, `queryTimeoutSeconds?: number`.
- Backend `NewDatasource` must use the passed `DataSourceInstanceSettings`,
  decrypt the token via `settings.DecryptedSecureJSONData["token"]`, and
  construct one `dynatrace.Client` per instance. The env-var path
  (`DT_TENANT_URL`, `DT_TOKEN`) is removed; settings are the single source of
  truth.
- Each instance is independent — two data sources in one Grafana pointed at
  different tenants must both work concurrently.

### R1.2 — Health check uses the configured tenant

- `CheckHealth` reports four distinct failure modes with actionable messages:
  - Tenant URL missing or malformed
  - Token missing
  - HTTP transport failure (DNS, TLS, connection refused) — include the host
  - DQL execute returned 4xx/5xx — surface the Grail error body
- On success, return `"Successfully connected to <host>"` and include the DQL
  API version if the response exposes it.

### R1.3 — Honor Grafana's time range

- Remove the hardcoded `v0Timeframe = 1 * time.Hour` constant in
  `pkg/plugin/datasource.go`.
- Pass `query.TimeRange.From` and `query.TimeRange.To` to
  `dynatrace.Client.Query` as `DefaultTimeframeStart` / `DefaultTimeframeEnd`.
- If the DQL string contains an explicit `from:` / `to:` / `timeframe:`
  clause, Grail will override the defaults — preserve this behavior, do not
  pre-parse the DQL.
- If the panel has no time range (variable queries, explore-time), fall back
  to the per-instance default from R1.1, else to "last 1h".

### R1.4 — Template variable interpolation

- Before sending the DQL to Grail, replace template variables using
  `getTemplateSrv().replace(dqlQuery, scopedVars, format)`.
- Choose `format = 'csv'` by default; document that users can pick another
  formatter with the `${var:json}` / `${var:singlequote}` syntax.
- Support the following built-in macros, expanded **server-side** in Go
  (a new `pkg/macros` package) so that backend-only contexts (alerting in
  Milestone 2) get the same substitutions:
  - `$__timeFilter()` / `$__timeFilter(<field>)` → `<field> >= <from> and <field> <= <to>`
    (Grail uses comparison operators, not a function call).
  - `$__from` → from epoch ms.
  - `$__to` → to epoch ms.
  - `$__fromTime` / `$__toTime` → ISO-8601 strings suitable for
    `from:"…", to:"…"`.
  - `$__interval` → seconds, mapped to the closest DQL `interval:` literal
    (`1s`, `5s`, `10s`, `30s`, `1m`, `5m`, …).
  - `$__interval_ms` → milliseconds as integer.
- Macro expansion runs **after** Grafana variable interpolation and is
  idempotent (a second pass on already-expanded DQL must be a no-op).

### R1.5 — Frame mapping for non-timeseries results

- Today `recordsToFrames` only handles records that contain a timestamp array
  and value arrays. After this milestone the mapper must also handle:
  - **Table shape** — every value in the record is a scalar; produce one wide
    frame, one column per record key, with field types inferred from the JSON
    value (`string`, `float64`, `bool`, time-string → `time.Time`).
  - **Array-of-scalars shape** — some columns are arrays of equal length but
    no timestamp column exists; produce a long frame (one row per array index).
- Detection precedence: timeseries (current code) → table → array-of-scalars
  → error.
- Errors on a single record must not abort the whole response: skip the
  record, attach a `data.Notice` with `Severity = Warning` to the response,
  and continue.
- Nested objects/maps in scalar fields are JSON-encoded to a string column
  rather than dropped silently.

### R1.6 — Value units and field display

- If a Grail record metadata block exposes a per-column unit (DQL
  `value.unit`, e.g. `Percent`, `MilliSecond`, `Byte`), map it to the
  Grafana unit string on `Field.Config.Unit`. A small lookup table is
  acceptable; unknown units fall back to no unit.
- Set `Field.Config.DisplayNameFromDS` from the column name when dimension
  labels are present, so multi-series panels render a useful legend.

### R1.7 — Unit tests for the mapper

- Add `pkg/plugin/frames_test.go` with fixtures captured from real Grail
  responses (timeseries with multiple dimensions, table from `fetch`,
  empty result, malformed record). Coverage target for `pkg/plugin/`:
  ≥ 70% lines.
- Add a frontend unit test that asserts `filterQuery` rejects empty / whitespace
  queries.

### R1.8 — Documentation rewrite

- Replace the scaffold `README.md` with:
  - One-paragraph description of what the plugin does and what it talks to.
  - Configuration steps (creating a platform token, required scopes, where
    to paste it in the UI).
  - Provisioning YAML example with `secureJsonData.token`.
  - Two example DQL queries (one `timeseries`, one `fetch`) with screenshots.
  - Known limitations (links forward to Milestones 2 and 3).
- Add a `CHANGELOG.md` entry summarizing the breaking change in R1.1
  (env-var auth removed).

## Definition of done

- Two data source instances pointing at different tenants coexist in one Grafana.
- A dashboard with a 24-hour time picker shows 24 hours of data (not 1 hour).
- A panel with a `$host` template variable substitutes correctly into the DQL.
- A `smartscapeNodes "HOST" | limit 10` query renders a table panel.
- `pkg/plugin/` test coverage ≥ 70%, all green in CI.
- README and CHANGELOG reviewed; provisioning example tested in
  `docker-compose.yaml`.
