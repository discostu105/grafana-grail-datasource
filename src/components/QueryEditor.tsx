import React, { ChangeEvent } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { InlineField, RadioButtonGroup, Input } from '@grafana/ui';
import { DataSource } from '../datasource';
import { DqlDataSourceOptions, DqlQuery, DqlQueryType } from '../types';

type Props = QueryEditorProps<DataSource, DqlQuery, DqlDataSourceOptions>;

const QUERY_TYPES: Array<{ label: string; value: DqlQueryType }> = [
  { label: 'Timeseries / Table', value: 'timeseries' },
  { label: 'Logs', value: 'logs' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const queryType: DqlQueryType = query.queryType ?? 'timeseries';

  const onDqlChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    onChange({ ...query, dqlQuery: event.target.value });
  };

  const onTypeChange = (value: DqlQueryType) => {
    onChange({ ...query, queryType: value });
    onRunQuery();
  };

  const onBodyFieldChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, logBodyField: event.target.value || undefined });
  };

  return (
    <div>
      <InlineField label="Query type" labelWidth={18}>
        <RadioButtonGroup options={QUERY_TYPES} value={queryType} onChange={onTypeChange} />
      </InlineField>
      {queryType === 'logs' && (
        <InlineField
          label="Body field"
          labelWidth={18}
          tooltip="Column carrying the log message. Defaults to `content` (DQL `fetch logs` default)."
        >
          <Input
            width={30}
            placeholder="content"
            value={query.logBodyField ?? ''}
            onChange={onBodyFieldChange}
          />
        </InlineField>
      )}
      <textarea
        aria-label="DQL"
        value={query.dqlQuery ?? ''}
        onChange={onDqlChange}
        onBlur={onRunQuery}
        placeholder={
          queryType === 'logs'
            ? 'fetch logs | filter $__timeFilter() | sort timestamp desc | limit 200'
            : 'timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}'
        }
        rows={8}
        style={{ width: '100%', fontFamily: 'monospace', padding: 8 }}
      />
    </div>
  );
}
