import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { InlineField, Input, SecretInput } from '@grafana/ui';
import { DqlDataSourceOptions, DqlSecureJsonData } from '../types';

type Props = DataSourcePluginOptionsEditorProps<DqlDataSourceOptions, DqlSecureJsonData>;

export function ConfigEditor({ options, onOptionsChange }: Props) {
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const onTenantUrlChange = (e: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, tenantUrl: e.target.value },
    });
  };

  const onApiTokenChange = (e: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: { ...(secureJsonData ?? {}), apiToken: e.target.value },
    });
  };

  const onApiTokenReset = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, apiToken: false },
      secureJsonData: { ...(secureJsonData ?? {}), apiToken: '' },
    });
  };

  return (
    <div style={{ maxWidth: 720 }}>
      <InlineField label="Tenant URL" labelWidth={20} tooltip="e.g. https://abc.apps.dynatrace.com">
        <Input
          width={50}
          placeholder="https://<env>.apps.dynatrace.com"
          value={jsonData.tenantUrl ?? ''}
          onChange={onTenantUrlChange}
        />
      </InlineField>
      <InlineField label="API token" labelWidth={20} tooltip="Platform token, e.g. dt0s16.…">
        <SecretInput
          width={50}
          placeholder="dt0s16.XXXX..."
          isConfigured={Boolean(secureJsonFields?.apiToken)}
          value={secureJsonData?.apiToken ?? ''}
          onChange={onApiTokenChange}
          onReset={onApiTokenReset}
        />
      </InlineField>
    </div>
  );
}
