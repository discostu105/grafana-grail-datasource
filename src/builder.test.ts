import { DEFAULT_BUILDER, dqlFromBuilder } from './builder';
import type { BuilderState } from './types';

function s(overrides: Partial<BuilderState> = {}): BuilderState {
  return { ...DEFAULT_BUILDER, ...overrides };
}

describe('dqlFromBuilder', () => {
  it('default (logs, count, bucket=auto) → makeTimeseries with no interval', () => {
    expect(dqlFromBuilder(DEFAULT_BUILDER)).toBe('fetch logs\n| makeTimeseries cnt = count()');
  });

  it('bucket=none → summarize, no makeTimeseries', () => {
    const dql = dqlFromBuilder(s({ bucket: 'none', groupBy: ['loglevel'] }));
    expect(dql).toBe('fetch logs\n| summarize cnt = count(), by:{loglevel}');
    expect(dql).not.toContain('makeTimeseries');
  });

  it('bucket=fixed → makeTimeseries with interval and by', () => {
    const dql = dqlFromBuilder(
      s({
        source: 'spans',
        aggregation: { fn: 'avg', field: 'duration' },
        groupBy: ['service.name'],
        bucket: '5m',
      })
    );
    expect(dql).toBe(
      'fetch spans\n| makeTimeseries result = avg(duration), interval: 5m, by:{service.name}'
    );
  });

  it('emits filter rows joined with lowercase and', () => {
    const dql = dqlFromBuilder(
      s({
        filters: [
          { field: 'host.name', operator: '==', value: 'h1' },
          { field: 'service.name', operator: '!=', value: 'noisy' },
        ],
      })
    );
    expect(dql).toContain('host.name == "h1"');
    expect(dql).toContain('service.name != "noisy"');
    expect(dql.split('| filter ')[1]).toContain(' and ');
    expect(dql).not.toContain(' AND ');
  });

  it('contains and matchesValue operators use the right DQL functions', () => {
    const dql = dqlFromBuilder(
      s({
        filters: [
          { field: 'content', operator: 'contains', value: 'oops' },
          { field: 'dt.tags', operator: 'matchesValue', value: '*prod*' },
        ],
      })
    );
    expect(dql).toContain('contains(content, "oops")');
    expect(dql).toContain('matchesValue(dt.tags, "*prod*")');
  });

  it('drops incomplete filter rows', () => {
    const dql = dqlFromBuilder(
      s({
        filters: [
          { field: '', operator: '==', value: 'x' },
          { field: 'k', operator: '==', value: '' },
          { field: 'k', operator: '==', value: 'v' },
        ],
      })
    );
    expect(dql).toContain('| filter k == "v"');
    expect(dql).not.toMatch(/\| filter[^|]* and /);
  });

  it('escapes embedded quotes in values', () => {
    const dql = dqlFromBuilder(s({ filters: [{ field: 'msg', operator: '==', value: 'has "quote"' }] }));
    expect(dql).toContain('msg == "has \\"quote\\""');
  });

  it('non-count aggregation defaults the field to duration', () => {
    const dql = dqlFromBuilder(s({ aggregation: { fn: 'avg' }, source: 'spans' }));
    expect(dql).toContain('avg(duration)');
  });

  it('metrics source emits timeseries, not fetch', () => {
    const dql = dqlFromBuilder(
      s({
        source: 'metrics',
        aggregation: { fn: 'avg', field: 'dt.host.cpu.usage' },
        groupBy: ['dt.smartscape.host'],
        bucket: 'auto',
      })
    );
    expect(dql).toBe('timeseries result = avg(`dt.host.cpu.usage`), by:{dt.smartscape.host}');
    expect(dql).not.toContain('fetch');
    expect(dql).not.toContain('makeTimeseries');
  });

  it('metrics + percentile adds rollup: avg and a default rank of 95', () => {
    const dql = dqlFromBuilder(
      s({
        source: 'metrics',
        aggregation: { fn: 'percentile', field: 'dt.service.request.response_time' },
        bucket: '5m',
      })
    );
    expect(dql).toBe(
      'timeseries result = percentile(`dt.service.request.response_time`, 95, rollup: avg), interval: 5m'
    );
  });

  it('metrics + count is coerced to avg (count of a metric is meaningless)', () => {
    const dql = dqlFromBuilder(s({ source: 'metrics', aggregation: { fn: 'count' } }));
    expect(dql).toContain('avg(`dt.host.cpu.usage`)');
    expect(dql).not.toContain('count(');
  });

  it('smartscapeNodes source is emitted verbatim (no fetch prefix)', () => {
    const dql = dqlFromBuilder(s({ source: 'smartscapeNodes "HOST"' }));
    expect(dql.startsWith('smartscapeNodes "HOST"')).toBe(true);
    expect(dql).not.toContain('fetch smartscapeNodes');
  });
});
