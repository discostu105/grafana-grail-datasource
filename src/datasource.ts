import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  DataSourceWithLogsContextSupport,
  DataSourceWithSupplementaryQueriesSupport,
  Field,
  LogRowContextOptions,
  LogRowContextQueryDirection,
  LogRowModel,
  MetricFindValue,
  ScopedVars,
  SupplementaryQueryOptions,
  SupplementaryQueryType,
  TimeRange,
} from '@grafana/data';
import { DataSourceWithBackend, getBackendSrv, getTemplateSrv } from '@grafana/runtime';
import { Observable, firstValueFrom, map } from 'rxjs';

import { AdhocFilter, DqlQuery, DqlDataSourceOptions, DEFAULT_QUERY } from './types';
import { GrailAutocompleteResponse } from './dql/language';
import { applyDerivedFields } from './derivedFields';
import { buildLogContextDQL, buildLogsVolumeQuery } from './logsHooks';
import { decodeTraceFrames, enhanceTraceListFrames, stampTraceCorrelations } from './tracesPostprocess';

// Curated tag keys we always expose for the ad-hoc filter UI. The Loxone
// tenant we've seen in practice carries control.name / control.category /
// control.room / state.name / unit on every metric series; entity-typed
// data uses dt.smartscape.host / host.name / service.name / k8s.namespace.name.
// These are merged with whatever Grail's autocomplete reports.
const CURATED_TAG_KEYS = [
  'control.name',
  'control.category',
  'control.room',
  'control.type',
  'state.name',
  'unit',
  'dt.smartscape.host',
  'dt.smartscape.service',
  'host.name',
  'service.name',
  'k8s.namespace.name',
];

// Shape of an ad-hoc variable's filter rows + the surrounding variable model.
// Mirrors @grafana/data's AdHocVariableModel; declared locally because
// TypedVariableModel is a discriminated union we can't extend, and we only
// need a structural subset for collection.
interface AdHocVariableFilterShape {
  key?: string;
  operator?: string;
  value?: unknown;
}

interface AdHocVariableShape {
  type: 'adhoc';
  filters?: AdHocVariableFilterShape[];
  datasource?: string | { uid?: string; type?: string } | null;
}

export class DataSource
  extends DataSourceWithBackend<DqlQuery, DqlDataSourceOptions>
  implements DataSourceWithSupplementaryQueriesSupport<DqlQuery>, DataSourceWithLogsContextSupport<DqlQuery>
{
  private readonly derivedFields?: DqlDataSourceOptions['derivedFields'];
  private readonly tracesToLogs?: DqlDataSourceOptions['tracesToLogs'];
  private readonly tracesToMetrics?: DqlDataSourceOptions['tracesToMetrics'];

  constructor(instanceSettings: DataSourceInstanceSettings<DqlDataSourceOptions>) {
    super(instanceSettings);
    this.derivedFields = instanceSettings.jsonData?.derivedFields;
    this.tracesToLogs = instanceSettings.jsonData?.tracesToLogs;
    this.tracesToMetrics = instanceSettings.jsonData?.tracesToMetrics;
  }

  getDefaultQuery() {
    return DEFAULT_QUERY;
  }

  // query is overridden so the response stream can be post-processed:
  //  - derived-field enhancement on log frames (regex matches on the body
  //    → clickable button column with DataLinks)
  //  - trace frames have their `tags` column JSON-decoded back into actual
  //    JS arrays so Grafana's traces panel doesn't choke on
  //    `K.reduce is not a function`.
  query(request: DataQueryRequest<DqlQuery>): Observable<DataQueryResponse> {
    // Index of the per-refid query model so we know what queryType each
    // returned frame was asked for — used by the trace-list enhancer to
    // attach clickable trace IDs to rollup table frames.
    const queryByRefId = new Map<string, DqlQuery>();
    for (const t of request.targets ?? []) {
      if (t?.refId) {
        queryByRefId.set(t.refId, t);
      }
    }
    return super.query(request).pipe(
      map((resp) => {
        let data = resp.data ?? [];
        if (this.derivedFields?.length) {
          data = applyDerivedFields(data, this.derivedFields);
        }
        data = decodeTraceFrames(data);
        data = stampTraceCorrelations(data, this.tracesToLogs, this.tracesToMetrics);
        data = enhanceTraceListFrames(data, this.uid, queryByRefId);
        return { ...resp, data };
      })
    );
  }

  // applyTemplateVariables runs once per query on the way to the backend:
  //  - $var / ${var:csv} interpolation (the backend does $__macros itself
  //    so alerting works without a templateSrv)
  //  - stamps the dashboard's ad-hoc filters onto the query so the backend
  //    can substitute $__adhocFilters / auto-append `| filter ...`
  applyTemplateVariables(query: DqlQuery, scopedVars: ScopedVars): DqlQuery {
    const dql = query.dqlQuery ? getTemplateSrv().replace(query.dqlQuery, scopedVars, 'csv') : query.dqlQuery;
    const adhocFilters = this.collectAdhocFilters();
    return { ...query, dqlQuery: dql, adhocFilters };
  }

  filterQuery(query: DqlQuery): boolean {
    return !!query.dqlQuery && query.dqlQuery.trim().length > 0;
  }

  // autocomplete hits the plugin's resource handler, which proxies Grail's
  // /platform/storage/query/v1/query:autocomplete. Exposed so the
  // QueryEditor's Monaco language registration can reuse it.
  autocomplete(query: string, position: number): Promise<GrailAutocompleteResponse> {
    return getBackendSrv().post(`/api/datasources/uid/${this.uid}/resources/autocomplete`, { query, position });
  }

  // collectAdhocFilters pulls the dashboard's ad-hoc filter variables that
  // target this datasource. Grafana stores them as variables of type 'adhoc'
  // with a `filters: Array<{key, operator, value}>` payload.
  private collectAdhocFilters(): AdhocFilter[] {
    const out: AdhocFilter[] = [];
    for (const v of getTemplateSrv().getVariables()) {
      if (v.type !== 'adhoc') {
        continue;
      }
      const adhoc = v as unknown as AdHocVariableShape;
      const ds = adhoc.datasource;
      // datasource can be a string (name/uid), {uid}, or null; match by uid first
      const dsUid = typeof ds === 'object' && ds !== null ? ds.uid : undefined;
      const matches = !ds || ds === this.name || ds === this.uid || dsUid === this.uid || dsUid === '$datasource';
      if (!matches) {
        continue;
      }
      for (const f of adhoc.filters ?? []) {
        if (!f?.key || f?.value == null) {
          continue;
        }
        out.push({ key: String(f.key), operator: String(f.operator ?? '='), value: String(f.value) });
      }
    }
    return out;
  }

  // ---- Ad-hoc filter discovery -------------------------------------------

  // getTagKeys is called by Grafana when the user opens the ad-hoc filter
  // dropdown for the first time. We return the curated list above merged
  // with anything Grail's autocomplete suggests for a `filter <cursor>`
  // position. Cheap: one autocomplete call, results are cached client-side
  // by Grafana.
  async getTagKeys(): Promise<MetricFindValue[]> {
    const keys = new Set<string>(CURATED_TAG_KEYS);
    try {
      const r = await this.autocomplete('fetch metric.series | filter ', 'fetch metric.series | filter '.length);
      for (const s of r.suggestions ?? []) {
        if (s.suggestion && (s.parts?.[0]?.type === 'FIELD' || s.parts?.[0]?.type === 'PARAMETER_KEY')) {
          keys.add(s.suggestion);
        }
      }
    } catch {
      // ignore — curated list is still useful
    }
    return [...keys].sort().map((text) => ({ text }));
  }

  // getTagValues is called when the user picks a key and is choosing a
  // value. Run `summarize by:{<key>}` against the metric.series catalogue
  // (cheap — metric.series is a metadata view, no data points scanned).
  async getTagValues(options: { key: string }): Promise<MetricFindValue[]> {
    if (!options?.key) {
      return [];
    }
    // The key may contain dots; DQL accepts them bare in `by:{}` clauses.
    const dql = `fetch metric.series | summarize count(), by:{${options.key}} | fields ${options.key} | sort ${options.key} asc | limit 1000`;
    const values = await this.metricFindQuery(dql);
    return values.map((v) => ({ text: String(v.text) }));
  }

  // ---- metricFindQuery (variable queries) --------------------------------

  async metricFindQuery(
    dql: string,
    options?: { variable?: { name: string }; range?: TimeRange }
  ): Promise<MetricFindValue[]> {
    if (!dql || !dql.trim()) {
      return [];
    }
    const refId = `metric-find-${options?.variable?.name ?? 'q'}`;
    const interpolated = getTemplateSrv().replace(dql, undefined, 'csv');

    const request: DataQueryRequest<DqlQuery> = {
      app: 'metric-find',
      requestId: refId,
      timezone: 'utc',
      interval: '1m',
      intervalMs: 60_000,
      startTime: Date.now(),
      scopedVars: {},
      targets: [{ refId, dqlQuery: interpolated } as DqlQuery],
      range: options?.range as TimeRange,
    };

    const response = await firstValueFrom(this.query(request));
    const frames = response?.data ?? [];
    if (!frames.length) {
      return [];
    }
    const frame = frames[0];
    const allFields = frame.fields ?? [];
    if (!allFields.length) {
      return [];
    }
    const nonTime = (allFields as Field[]).filter((f) => (f.name ?? '').toLowerCase() !== 'time');
    if (!nonTime.length) {
      return [];
    }
    const fieldByName = (name: string) => nonTime.find((f) => (f.name ?? '').toLowerCase() === name.toLowerCase());

    const textField = fieldByName('text') ?? nonTime[0];
    const valueField = fieldByName('value') ?? textField;
    const len = textField.values?.length ?? 0;

    // Grafana frames historically exposed values as a Vector with a get()
    // method; newer versions use plain arrays. Handle both.
    const cellAt = (field: { values?: unknown }, i: number): unknown => {
      const vs = field.values as { get?: (i: number) => unknown } | unknown[] | undefined;
      if (vs && typeof (vs as { get?: unknown }).get === 'function') {
        return (vs as { get: (i: number) => unknown }).get(i);
      }
      return (vs as unknown[] | undefined)?.[i];
    };

    const seen = new Set<string>();
    const out: MetricFindValue[] = [];
    for (let i = 0; i < len; i++) {
      const t = cellAt(textField, i);
      if (t == null || t === '') {
        continue;
      }
      const text = String(t);
      const vRaw = cellAt(valueField, i);
      const value = typeof vRaw === 'number' ? vRaw : String(vRaw ?? text);
      const key = `${text}|${value}`;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      out.push({ text, value });
    }
    return out;
  }

  // ---- Log volume histogram (DataSourceWithSupplementaryQueriesSupport) ---
  //
  // Grafana asks the data source for a "supplementary" query alongside the
  // main one when rendering the logs panel. Returning a LogsVolume query
  // here drives the bar chart Grafana displays above the log lines, broken
  // down by severity. Pattern matches grafana-clickhouse-datasource.

  getSupportedSupplementaryQueryTypes(): SupplementaryQueryType[] {
    return [SupplementaryQueryType.LogsVolume];
  }

  getSupplementaryQuery(options: SupplementaryQueryOptions, query: DqlQuery): DqlQuery | undefined {
    return buildLogsVolumeQuery(options.type, query);
  }

  getDataProvider(
    type: SupplementaryQueryType,
    request: DataQueryRequest<DqlQuery>
  ): Observable<DataQueryResponse> | undefined {
    if (type !== SupplementaryQueryType.LogsVolume) {
      return undefined;
    }
    const targets = request.targets
      .map((t) => this.getSupplementaryQuery({ type }, t))
      .filter((t): t is DqlQuery => !!t);
    if (!targets.length) {
      return undefined;
    }
    return this.query({ ...request, targets });
  }

  // ---- Log row context (DataSourceWithLogsContextSupport) ----------------

  // showContextToggle hides the "show context" button when there's nothing
  // to anchor against. We require at least one label on the row (otherwise
  // the surrounding-lines query would be unfiltered and useless).
  showContextToggle(row?: LogRowModel): boolean {
    return !!row?.labels && Object.keys(row.labels).length > 0;
  }

  async getLogRowContext(
    row: LogRowModel,
    options?: LogRowContextOptions,
    origQuery?: DqlQuery
  ): Promise<DataQueryResponse> {
    const { dql, fromMs, toMs } = buildLogContextDQL(row, options);
    const direction = options?.direction === LogRowContextQueryDirection.Forward ? 'asc' : 'desc';
    const refId = `${origQuery?.refId ?? 'A'}-ctx-${direction}`;
    const request: DataQueryRequest<DqlQuery> = {
      app: 'logs-context',
      requestId: refId,
      timezone: 'utc',
      interval: '1m',
      intervalMs: 60_000,
      startTime: Date.now(),
      scopedVars: {},
      targets: [{ refId, dqlQuery: dql, queryType: 'logs' } as DqlQuery],
      range: {
        // Grafana's TimeRange has .from/.to as Dayjs/Moment-like objects;
        // we expose .valueOf() returning ms which is what the backend
        // proxy uses. The raw strings are unused for log-context queries.
        from: { valueOf: () => fromMs } as unknown as TimeRange['from'],
        to: { valueOf: () => toMs } as unknown as TimeRange['to'],
        raw: { from: '', to: '' },
      },
    };
    return firstValueFrom(this.query(request));
  }
}
