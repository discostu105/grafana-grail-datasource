// Take a screenshot of Grafana Explore rendering a real trace via our plugin.
import { chromium } from '@playwright/test';

const URL = 'https://grafana.int.neumueller.net';
const PASS = process.env.PASS;
const DS_UID = 'P6C323D126547F71F';

const browser = await chromium.launch({ headless: true, args: ['--ignore-certificate-errors'] });
const ctx = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1920, height: 1200 } });
const page = await ctx.newPage();
const consoleErrs = [];
page.on('console', (m) => {
  if (['error', 'warning'].includes(m.type())) {
    consoleErrs.push(`${m.type()}: ${m.text().slice(0, 300)}`);
  }
});
page.on('pageerror', (e) => consoleErrs.push(`pageerror: ${e.message}`));

await page.request.post(`${URL}/login`, {
  data: { user: 'admin', password: PASS },
  headers: { 'Content-Type': 'application/json' },
});

const left = {
  datasource: DS_UID,
  queries: [
    {
      refId: 'A',
      datasource: { uid: DS_UID, type: 'discostu105-grail-datasource' },
      dqlQuery: 'fetch spans | filter service.name == "homelab-telemetrygen" | limit 8 | sort start_time',
      queryType: 'traces',
    },
  ],
  range: { from: 'now-5m', to: 'now' },
};
const target = `${URL}/explore?orgId=1&left=${encodeURIComponent(JSON.stringify(left))}`;
console.error('navigating:', target);

try {
  await page.goto(target, { waitUntil: 'networkidle', timeout: 45000 });
} catch (e) {
  console.error('nav:', e.message);
}
await page.waitForTimeout(12000);

// Capture the actual response frame so we can see what Grafana receives
// — proves whether our decodeTraceFrames postprocess fired.
const responses = [];
page.on('response', async (r) => {
  if (r.url().includes('/api/ds/query')) {
    try {
      const body = await r.json();
      responses.push(body);
    } catch {}
  }
});
// Trigger one more run so the listener catches it.
await page
  .getByRole('button', { name: /Run query/i })
  .click()
  .catch(() => {});
await page.waitForTimeout(8000);
if (responses.length) {
  const fr = responses[responses.length - 1]?.results?.A?.frames?.[0];
  if (fr) {
    const fields = fr.schema.fields.map((f) => `${f.name}:${f.type}`);
    console.error('  fields:', fields.join(', '));
    const tagsIdx = fr.schema.fields.findIndex((f) => f.name === 'tags');
    if (tagsIdx >= 0) {
      const sample = fr.data.values[tagsIdx]?.[0];
      console.error('  tags row 0 type:', typeof sample, 'preview:', JSON.stringify(sample).slice(0, 200));
    }
  }
}

// Click "Details" on the unexpected-error toast to expand it.
try {
  await page.getByText('Details').first().click({ timeout: 3000 });
  await page.waitForTimeout(1000);
} catch {
  // no toast — fine
}

// Try clicking a span row in the waterfall — this is the path that
// previously triggered Cannot read properties of undefined (toLowerCase).
try {
  await page.getByText('okey-dokey-0', { exact: false }).first().click({ timeout: 5000 });
  await page.waitForTimeout(3000);
  console.error('  clicked a span');
} catch (e) {
  console.error('  could not click span:', e.message.slice(0, 100));
}

await page.screenshot({ path: '/tmp/trace-view.png', fullPage: true });

const bodyText = await page.evaluate(() => document.body.innerText);
const errSection = bodyText.match(/An unexpected error[\s\S]{0,1500}/);
if (errSection) {
  console.error('\nerror toast text:\n' + errSection[0]);
}

// Console errors are usually more informative.
page.on('console', () => {}); // suppress further

console.error('\nconsole errs (' + consoleErrs.length + '):');
for (const e of consoleErrs.slice(0, 5)) {
  console.error('  ', e);
}

await browser.close();
console.error('done — see /tmp/trace-view.png');
