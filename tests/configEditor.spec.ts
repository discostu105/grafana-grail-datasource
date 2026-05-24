import { test, expect } from '@grafana/plugin-e2e';

test('smoke: should render tenant URL + API token fields', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await createDataSourceConfigPage({ type: ds.type });
  await expect(page.getByText('Tenant URL')).toBeVisible();
  await expect(page.getByText('API token')).toBeVisible();
});
