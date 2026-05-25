import React, { ChangeEvent } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
// (autocomplete now lives entirely in src/dql/language.ts; QueryEditor just
// proxies the resource call through to the language registration.)
import { CodeEditor, InlineField, RadioButtonGroup, Input, Button, HorizontalGroup } from '@grafana/ui';
import { DataSource } from '../datasource';
import { DqlDataSourceOptions, DqlQuery, DqlQueryType } from '../types';
import { DQL_LANGUAGE_ID, registerDqlLanguage } from '../dql/language';

type Props = QueryEditorProps<DataSource, DqlQuery, DqlDataSourceOptions>;

const QUERY_TYPES: Array<{ label: string; value: DqlQueryType }> = [
  { label: 'Timeseries / Table', value: 'timeseries' },
  { label: 'Logs', value: 'logs' },
];

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const queryType: DqlQueryType = query.queryType ?? 'timeseries';

  const onDqlChange = (value: string) => {
    onChange({ ...query, dqlQuery: value });
  };

  const onTypeChange = (value: DqlQueryType) => {
    onChange({ ...query, queryType: value });
    onRunQuery();
  };

  const onBodyFieldChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, logBodyField: event.target.value || undefined });
  };

  // Proxy the Monaco completion provider to the plugin's
  // /resources/autocomplete endpoint, which in turn proxies Grail's
  // /platform/storage/query/v1/query:autocomplete.
  const autocomplete = async (dql: string, position: number) => {
    return getBackendSrv().post(
      `/api/datasources/uid/${datasource.uid}/resources/autocomplete`,
      { query: dql, position }
    );
  };

  return (
    <div>
      <HorizontalGroup spacing="sm" align="center">
        <InlineField label="Query type" labelWidth={18}>
          <RadioButtonGroup options={QUERY_TYPES} value={queryType} onChange={onTypeChange} />
        </InlineField>
        <Button size="sm" variant="secondary" onClick={onRunQuery} icon="play">
          Run
        </Button>
      </HorizontalGroup>
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
      {queryType !== 'logs' && (
        <InlineField
          label="Legend"
          labelWidth={18}
          tooltip="Optional series name template. Use {{ control.name }} or ${__field.labels.control.name}. Leave blank for the default (the most relevant label, e.g. control.name)."
        >
          <Input
            width={40}
            placeholder="{{ control.name }}"
            value={query.legendFormat ?? ''}
            onChange={(e: ChangeEvent<HTMLInputElement>) =>
              onChange({ ...query, legendFormat: e.target.value || undefined })
            }
          />
        </InlineField>
      )}
      <div style={{ marginTop: 8 }}>
        <CodeEditor
          value={query.dqlQuery ?? ''}
          language={DQL_LANGUAGE_ID}
          height={180}
          showLineNumbers
          showMiniMap={false}
          onBlur={onDqlChange}
          onSave={(v) => {
            onDqlChange(v);
            onRunQuery();
          }}
          onBeforeEditorMount={(monaco) => {
            registerDqlLanguage(monaco, autocomplete);
          }}
          onEditorDidMount={(editor, monaco) => {
            editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
              onDqlChange(editor.getValue());
              onRunQuery();
            });
          }}
          monacoOptions={{
            scrollBeyondLastLine: false,
            wordWrap: 'on',
            fontFamily: 'monospace',
          }}
        />
      </div>
    </div>
  );
}
