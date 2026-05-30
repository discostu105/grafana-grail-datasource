import { test, expect } from '@grafana/plugin-e2e';
import { SELECTORS } from '../src/selectors';

const Q = SELECTORS.queryEditor;

test('smoke: renders Monaco DQL editor and query-type radio', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);

  // The Monaco container renders as a [role=code] (textbox in older Monaco
  // builds) inside @grafana/ui's CodeEditor wrapper. Check both query-type
  // options + the legend input are visible.
  await expect(page.getByText(Q.queryTypeLabel)).toBeVisible();
  await expect(page.getByRole('radio', { name: Q.queryTypeRadios.timeseries })).toBeVisible();
  await expect(page.getByRole('radio', { name: Q.queryTypeRadios.logs })).toBeVisible();
  // Scope "Legend" to the query editor row: the panel-options pane also
  // renders "Legend" sections, so a page-wide getByText is ambiguous
  // (strict-mode "resolved to N elements") and flakes on render timing.
  await expect(panelEditPage.getQueryEditorRow('A').getByText(Q.legendLabel)).toBeVisible();
  // CodeEditor renders a textarea in the shadow DOM; assert the container.
  await expect(panelEditPage.panel.locator).toBeVisible();
});

test('switches to Logs mode → Body field appears, Legend disappears', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);

  await page.getByRole('radio', { name: Q.queryTypeRadios.logs }).click();

  await expect(page.getByText(Q.bodyFieldLabel)).toBeVisible();
  await expect(panelEditPage.getQueryEditorRow('A').getByText(Q.legendLabel)).not.toBeVisible();
});
