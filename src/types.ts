import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type DqlQueryType = 'timeseries' | 'logs';

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
}

export interface DqlSecureJsonData {
  apiToken?: string;
}
