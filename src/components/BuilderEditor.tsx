import React, { ChangeEvent, useEffect, useMemo, useState } from 'react';
import { Button, Combobox, Field as UiField, Input, Select, Stack } from '@grafana/ui';
import type { BuilderState, BuilderFilter, BuilderOperator, BuilderAggFn, BuilderBucket, DataObject } from '../types';
import { BUILDER_AGG_FNS, BUILDER_BUCKETS, BUILDER_OPERATORS, BUILDER_SOURCES } from '../builder';

interface Props {
  value: BuilderState;
  onChange: (next: BuilderState) => void;
  // Optional loader; QueryEditor wires this to DataSource.dataObjects().
  // When missing or rejecting, the dropdown falls back to BUILDER_SOURCES.
  loadDataObjects?: () => Promise<DataObject[]>;
}

// Sources that aren't fetchable tables and so never appear in
// `fetch dt.system.data_objects`. The builder still supports them via
// command-specific generation paths: `metrics` → `timeseries`,
// `smartscapeNodes "HOST"` / `"SERVICE"` → top-level smartscapeNodes command.
// Pinned at the top of the dropdown so they're discoverable.
const SYNTHETIC_OPTIONS = [
  { label: 'metrics (timeseries)', value: 'metrics' },
  { label: 'smartscapeNodes "HOST"', value: 'smartscapeNodes "HOST"' },
  { label: 'smartscapeNodes "SERVICE"', value: 'smartscapeNodes "SERVICE"' },
];

export function BuilderEditor({ value, onChange, loadDataObjects }: Props) {
  const update = (patch: Partial<BuilderState>) => onChange({ ...value, ...patch });
  const [tables, setTables] = useState<DataObject[] | null>(null);

  useEffect(() => {
    if (!loadDataObjects) {
      return;
    }
    let alive = true;
    loadDataObjects()
      .then((rows) => {
        if (alive) {
          setTables(rows);
        }
      })
      .catch(() => {
        // Silently fall back — BUILDER_SOURCES still works.
      });
    return () => {
      alive = false;
    };
  }, [loadDataObjects]);

  const sourceOptions = useMemo(() => {
    // When the live list is available, prefer it; otherwise fall back to
    // the curated BUILDER_SOURCES we ship in the bundle. Synthetic entries
    // (metrics, smartscapeNodes) are always pinned at the top.
    const synthetic = new Set(SYNTHETIC_OPTIONS.map((o) => o.value));
    const rows =
      tables && tables.length > 0
        ? tables.map((t) => ({ label: `${t.display_name} (${t.name})`, value: t.name }))
        : BUILDER_SOURCES.filter((s) => !synthetic.has(s)).map((s) => ({ label: s, value: s }));
    return [...SYNTHETIC_OPTIONS, ...rows];
  }, [tables]);

  return (
    <Stack direction="column" gap={1}>
      <Stack direction="row" gap={2} alignItems="flex-end">
        <UiField label="Data source" description="Grail table (live from dt.system.data_objects)">
          <Combobox
            width={36}
            options={sourceOptions}
            value={{
              label: sourceOptions.find((o) => o.value === value.source)?.label ?? value.source,
              value: value.source,
            }}
            onChange={(opt) => update({ source: opt?.value ?? value.source })}
            createCustomValue
          />
        </UiField>
        <UiField label="Aggregation">
          <Select
            width={16}
            options={BUILDER_AGG_FNS.map((f) => ({ label: f, value: f }))}
            value={value.aggregation.fn}
            onChange={(opt) =>
              update({ aggregation: { ...value.aggregation, fn: (opt?.value ?? 'count') as BuilderAggFn } })
            }
          />
        </UiField>
        {value.source === 'metrics' ? (
          // Metrics aggregate a metric key (e.g. dt.host.cpu.usage), not an
          // event field — so this is always shown, even for count (which the
          // generator coerces to avg). Without it the DQL falls back to a
          // hardcoded default metric.
          <UiField label="Metric key" description="Dynatrace metric key to aggregate">
            <Input
              width={28}
              placeholder="dt.host.cpu.usage"
              value={value.aggregation.field ?? ''}
              onChange={(e: ChangeEvent<HTMLInputElement>) =>
                update({ aggregation: { ...value.aggregation, field: e.target.value || undefined } })
              }
            />
          </UiField>
        ) : (
          value.aggregation.fn !== 'count' && (
            <UiField label="Field" description="Numeric field to aggregate">
              <Input
                width={20}
                placeholder="duration"
                value={value.aggregation.field ?? ''}
                onChange={(e: ChangeEvent<HTMLInputElement>) =>
                  update({ aggregation: { ...value.aggregation, field: e.target.value || undefined } })
                }
              />
            </UiField>
          )
        )}
        <UiField label="Time bucket" description="Group by binned timestamp">
          <Select
            width={14}
            options={BUILDER_BUCKETS.map((b) => ({ label: b, value: b }))}
            value={value.bucket}
            onChange={(opt) => update({ bucket: (opt?.value ?? 'auto') as BuilderBucket })}
          />
        </UiField>
      </Stack>

      <FiltersList value={value.filters} onChange={(filters) => update({ filters })} />
      <GroupByList value={value.groupBy} onChange={(groupBy) => update({ groupBy })} />
    </Stack>
  );
}

function FiltersList({ value, onChange }: { value: BuilderFilter[]; onChange: (next: BuilderFilter[]) => void }) {
  const add = () => onChange([...value, { field: '', operator: '==', value: '' }]);
  const remove = (idx: number) => onChange(value.filter((_, i) => i !== idx));
  const update = (idx: number, patch: Partial<BuilderFilter>) =>
    onChange(value.map((f, i) => (i === idx ? { ...f, ...patch } : f)));

  return (
    <Stack direction="column" gap={1}>
      <span style={{ fontSize: 12, color: 'var(--theme-colors-text-secondary, #888)' }}>Filters</span>
      {value.map((f, i) => (
        <Stack key={i} direction="row" gap={1} alignItems="flex-end">
          <UiField label="Field">
            <Input
              width={22}
              placeholder="host.name"
              value={f.field}
              onChange={(e: ChangeEvent<HTMLInputElement>) => update(i, { field: e.target.value })}
            />
          </UiField>
          <UiField label="Op">
            <Select
              width={14}
              options={BUILDER_OPERATORS.map((o) => ({ label: o, value: o }))}
              value={f.operator}
              onChange={(opt) => update(i, { operator: (opt?.value ?? '==') as BuilderOperator })}
            />
          </UiField>
          <UiField label="Value">
            <Input
              width={24}
              placeholder="prod-1"
              value={f.value}
              onChange={(e: ChangeEvent<HTMLInputElement>) => update(i, { value: e.target.value })}
            />
          </UiField>
          <Button variant="destructive" size="sm" icon="trash-alt" onClick={() => remove(i)}>
            Remove
          </Button>
        </Stack>
      ))}
      <Button variant="secondary" size="sm" icon="plus" onClick={add} style={{ alignSelf: 'flex-start' }}>
        Add filter
      </Button>
    </Stack>
  );
}

function GroupByList({ value, onChange }: { value: string[]; onChange: (next: string[]) => void }) {
  const add = () => onChange([...value, '']);
  const remove = (idx: number) => onChange(value.filter((_, i) => i !== idx));
  const update = (idx: number, v: string) => onChange(value.map((x, i) => (i === idx ? v : x)));

  return (
    <Stack direction="column" gap={1}>
      <span style={{ fontSize: 12, color: 'var(--theme-colors-text-secondary, #888)' }}>Group by</span>
      {value.map((g, i) => (
        <Stack key={i} direction="row" gap={1} alignItems="flex-end">
          <UiField label={`Dimension ${i + 1}`}>
            <Input
              width={28}
              placeholder="service.name"
              value={g}
              onChange={(e: ChangeEvent<HTMLInputElement>) => update(i, e.target.value)}
            />
          </UiField>
          <Button variant="destructive" size="sm" icon="trash-alt" onClick={() => remove(i)}>
            Remove
          </Button>
        </Stack>
      ))}
      <Button variant="secondary" size="sm" icon="plus" onClick={add} style={{ alignSelf: 'flex-start' }}>
        Add group-by dimension
      </Button>
    </Stack>
  );
}
