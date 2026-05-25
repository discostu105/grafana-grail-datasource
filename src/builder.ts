// Pure DQL generator for the visual query builder.
//
// One-way: BuilderState → DQL string. We don't try to parse arbitrary
// hand-written DQL back into a BuilderState — switching code→builder
// in the editor warns before overwriting unsaved manual edits.
//
// Command selection follows DQL semantics (see dt-dql-essentials):
//   - source=metrics, bucket≠none      → `timeseries` (queries pre-ingested metric data)
//   - non-metric source, bucket=none   → `fetch X | summarize` (table aggregation)
//   - non-metric source, bucket≠none   → `fetch X | makeTimeseries` (event-based timeseries)
//
// `summarize ... by:{bin(timestamp, …)}` is NOT how DQL buckets time —
// that was a SQL-flavoured mistake. Use makeTimeseries.

import type {
  BuilderAggFn,
  BuilderBucket,
  BuilderFilter,
  BuilderOperator,
  BuilderSource,
  BuilderState,
} from './types';

export const DEFAULT_BUILDER: BuilderState = {
  source: 'logs',
  filters: [],
  groupBy: [],
  aggregation: { fn: 'count' },
  bucket: 'auto',
};

export const BUILDER_SOURCES: BuilderSource[] = [
  'logs',
  'spans',
  'events',
  'bizevents',
  'dt.davis.problems',
  'dt.davis.events',
  'metrics',
  'smartscapeNodes "HOST"',
  'smartscapeNodes "SERVICE"',
];

export const BUILDER_OPERATORS: BuilderOperator[] = ['==', '!=', 'contains', 'matchesValue'];

export const BUILDER_AGG_FNS: BuilderAggFn[] = [
  'count',
  'avg',
  'sum',
  'min',
  'max',
  'median',
  'percentile',
];

export const BUILDER_BUCKETS: BuilderBucket[] = ['none', 'auto', '1m', '5m', '15m', '1h'];

export function dqlFromBuilder(b: BuilderState): string {
  return b.source === 'metrics' ? dqlFromMetrics(b) : dqlFromFetch(b);
}

function dqlFromFetch(b: BuilderState): string {
  const filters = b.filters.filter((f) => f.field?.trim() && f.value?.trim());
  const groupBy = b.groupBy.filter(Boolean);
  const lines: string[] = [b.source.startsWith('smartscapeNodes ') ? b.source : `fetch ${b.source}`];
  if (filters.length) {
    lines.push(`| filter ${filters.map(formatFilter).join(' and ')}`);
  }

  const aggExpr = fetchAggExpr(b.aggregation);
  const byClause = groupBy.length ? `, by:{${groupBy.join(', ')}}` : '';

  if (b.bucket === 'none') {
    lines.push(`| summarize ${aggExpr}${byClause}`);
  } else {
    const interval = b.bucket === 'auto' ? '' : `, interval: ${b.bucket}`;
    lines.push(`| makeTimeseries ${aggExpr}${interval}${byClause}`);
  }
  return lines.join('\n');
}

function dqlFromMetrics(b: BuilderState): string {
  // `timeseries` operates on pre-ingested metrics, so `count()` is degenerate
  // (the count metric of a count metric is meaningless). Coerce to avg.
  const fn: BuilderAggFn = b.aggregation.fn === 'count' ? 'avg' : b.aggregation.fn;
  const metricKey = b.aggregation.field?.trim() || 'dt.host.cpu.usage';
  // Hyphenated metric keys need backticks per DQL. We backtick every key —
  // harmless for plain dotted names.
  const keyRef = `\`${metricKey}\``;

  // percentile / median / percentRank require rollup: on gauge/count metrics;
  // without it the query silently returns empty results.
  let aggCall: string;
  if (fn === 'percentile') {
    aggCall = `percentile(${keyRef}, 95, rollup: avg)`;
  } else if (fn === 'median') {
    aggCall = `median(${keyRef}, rollup: avg)`;
  } else {
    aggCall = `${fn}(${keyRef})`;
  }

  const groupBy = b.groupBy.filter(Boolean);
  const byClause = groupBy.length ? `, by:{${groupBy.join(', ')}}` : '';
  // 'auto' and 'none' both mean "let the engine pick" for `timeseries`.
  const interval = b.bucket === 'auto' || b.bucket === 'none' ? '' : `, interval: ${b.bucket}`;
  return `timeseries result = ${aggCall}${interval}${byClause}`;
}

function fetchAggExpr(a: BuilderState['aggregation']): string {
  if (a.fn === 'count') {
    return 'cnt = count()';
  }
  const field = a.field?.trim() || 'duration';
  if (a.fn === 'percentile') {
    return `result = percentile(${field}, 95)`;
  }
  return `result = ${a.fn}(${field})`;
}

function formatFilter(f: BuilderFilter): string {
  const v = escapeDQL(f.value.trim());
  switch (f.operator) {
    case '!=':
      return `${f.field} != "${v}"`;
    case 'contains':
      // Substring match on string fields.
      return `contains(${f.field}, "${v}")`;
    case 'matchesValue':
      // For array fields (dt.tags) or wildcard patterns ("*prod*").
      // On scalar strings without wildcards it does an exact match,
      // not substring — the UI hint should mention this.
      return `matchesValue(${f.field}, "${v}")`;
    case '==':
    default:
      return `${f.field} == "${v}"`;
  }
}

function escapeDQL(s: string): string {
  return s.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}
