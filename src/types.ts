import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export interface DqlQuery extends DataQuery {
  dqlQuery: string;
}

export const DEFAULT_QUERY: Partial<DqlQuery> = {
  dqlQuery: '',
};

export interface DqlDataSourceOptions extends DataSourceJsonData {
  tenantUrl?: string;
}

export interface DqlSecureJsonData {
  apiToken?: string;
}
