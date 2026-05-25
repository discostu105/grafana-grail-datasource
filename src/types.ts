import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type DqlQueryType = 'timeseries' | 'logs' | 'traces';

export type EditorMode = 'code' | 'builder';

// Sources map to DQL `fetch <X>` data objects, except `metrics` which
// switches the generator over to the `timeseries` command (which reads
// pre-ingested metrics rather than scanning events). `dt.entity.*` is
// deprecated in DQL — use `dt.smartscape.*` if topology is needed.
export type BuilderSource =
  | 'logs'
  | 'spans'
  | 'events'
  | 'bizevents'
  | 'dt.davis.problems'
  | 'dt.davis.events'
  | 'metrics'
  | 'smartscapeNodes "HOST"'
  | 'smartscapeNodes "SERVICE"';

// `matchesValue` is for array fields (dt.tags) or wildcard patterns ("*prod*").
// Use `contains` for substring search on plain strings.
export type BuilderOperator = '==' | '!=' | 'contains' | 'matchesValue';

export type BuilderAggFn = 'count' | 'avg' | 'sum' | 'min' | 'max' | 'median' | 'percentile';

// `none`  → emit `summarize`, no time bucket (good for tables)
// `auto`  → emit `makeTimeseries` / `timeseries` without `interval:` (engine picks)
// fixed   → emit explicit `interval: 5m` etc.
export type BuilderBucket = 'none' | 'auto' | '1m' | '5m' | '15m' | '1h';

export interface BuilderFilter {
  field: string;
  operator: BuilderOperator;
  value: string;
}

export interface BuilderState {
  source: BuilderSource | string;
  filters: BuilderFilter[];
  groupBy: string[];
  aggregation: { fn: BuilderAggFn; field?: string };
  bucket: BuilderBucket;
}

export interface DqlQuery extends DataQuery {
  dqlQuery: string;
  // Default 'timeseries'. When 'logs', the backend maps records to a logs
  // frame (Meta.PreferredVisualisation=logs, time/body/level/labels).
  queryType?: DqlQueryType;
  // Override the column that carries the log body when queryType=logs.
  // Defaults to 'content' (DQL `fetch logs` default).
  logBodyField?: string;
  // Optional Grafana legend template for the value series, e.g.
  // "{{ control.name }} (avg)". Mapped to Field.Config.DisplayName on the
  // backend; Grafana then resolves ${__field.labels.X} expressions itself.
  legendFormat?: string;
  // Ad-hoc filters from the dashboard's top bar, stamped onto the query
  // by applyTemplateVariables() on the way to the backend. The backend
  // substitutes $__adhocFilters or auto-appends `| filter ...`.
  adhocFilters?: AdhocFilter[];
  // Visual builder UX state. When the editor is in builder mode every
  // change writes through to dqlQuery via dqlFromBuilder(); when in
  // code mode dqlQuery is the source of truth and builder lags.
  editorMode?: EditorMode;
  builder?: BuilderState;
}

export interface AdhocFilter {
  key: string;
  operator: string; // "=", "!=", "=~", "!~"
  value: string;
}

export const DEFAULT_QUERY: Partial<DqlQuery> = {
  dqlQuery: '',
  queryType: 'timeseries',
};

export interface DqlDataSourceOptions extends DataSourceJsonData {
  tenantUrl?: string;
  queryTimeoutSeconds?: number;
  // Go duration string used when the panel has no time range (variable
  // queries, alerting probes). Defaults to "1h" on the backend.
  defaultTimeframe?: string;
  // Derived field rules — regex over log body → clickable URL link.
  // Each rule produces a new field on the logs frame whose values are
  // the first regex capture group; Grafana's logs panel renders them as
  // buttons that open the URL with ${__value.raw} substituted.
  derivedFields?: DerivedField[];
  // Trace-to-logs / trace-to-metrics correlation. Stamped onto trace
  // frames' Meta.Custom so Grafana's TraceView renders Span → Logs /
  // Span → Metrics buttons. ${__span.traceId} / ${__span.spanId} are
  // substituted by Grafana at click time.
  tracesToLogs?: TracesToLogsConfig;
  tracesToMetrics?: TracesToMetricsConfig;
}

export interface TracesToLogsConfig {
  // UID of the target datasource (often this plugin's own uid for
  // self-referential trace→DQL log correlation).
  datasourceUid?: string;
  // DQL template. ${__span.traceId} / ${__span.spanId} get substituted.
  query?: string;
  // Default: span time +/- 1h; only used when query has no $__timeFilter().
  // Leaving optional for forward-compat with Grafana's contract.
  spanStartTimeShift?: string;
  spanEndTimeShift?: string;
}

export interface TracesToMetricsConfig {
  datasourceUid?: string;
  // Multiple named queries — Grafana shows one button per entry.
  queries?: Array<{ name: string; query: string }>;
}

export interface DerivedField {
  // Name of the new field added to the logs frame. Shown as the button
  // label in the logs detail view (overridden by `urlDisplayLabel` if set).
  name: string;
  // Regex applied against each row's body. The first capture group is
  // the extracted value. Patterns without a capture group fall back to
  // the whole match.
  matcherRegex: string;
  // URL template. ${__value.raw} is replaced with the captured value;
  // standard Grafana template vars also work.
  url: string;
  // Optional button label override. Defaults to `name`.
  urlDisplayLabel?: string;
  // Optional Grafana datasource UID — when set, the link becomes an
  // "internal" link that opens Explore against that datasource.
  datasourceUid?: string;
}

export interface DqlSecureJsonData {
  apiToken?: string;
}
