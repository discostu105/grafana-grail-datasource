// Post-process trace frames so Grafana's traces panel doesn't choke.
//
// The backend emits these columns as JSON-encoded strings (cheap wire
// format): `tags`, `serviceTags`, `logs`, `references`. The panel calls
// `.map()` / `.reduce()` / `.forEach()` on them directly, so they need
// to be real JS arrays by the time the frame reaches the renderer.
//
// We mutate the existing frame in place: field.values gets replaced
// with the parsed array, field.type swaps to `other`. That's simpler
// and more robust than reconstructing the frame via MutableDataFrame
// (which has its own opinions about how to interpret the input).

import { DataFrame, Field, FieldType } from '@grafana/data';

const TRACE_VIS = 'trace';
const ARRAY_FIELDS = new Set(['tags', 'serviceTags', 'logs', 'references']);

export function decodeTraceFrames(frames: DataFrame[]): DataFrame[] {
  if (!frames?.length) {
    return frames;
  }
  for (const frame of frames) {
    if (frame.meta?.preferredVisualisationType !== TRACE_VIS) {
      continue;
    }
    decodeArrayFieldsInPlace(frame);
  }
  return frames;
}

function decodeArrayFieldsInPlace(frame: DataFrame): void {
  for (const f of frame.fields) {
    if (!ARRAY_FIELDS.has(f.name)) {
      continue;
    }
    const n = f.values?.length ?? 0;
    const decoded: unknown[] = new Array(n);
    for (let i = 0; i < n; i++) {
      const cell = readCell(f, i);
      decoded[i] = parseArrayCell(cell);
    }
    // Direct in-place mutation. The Vector / array on the values field
    // is swapped out, the type is updated. Grafana's frame model is
    // structural — no internal indexes to rebuild.
    (f as unknown as { values: unknown[] }).values = decoded;
    (f as unknown as { type: FieldType }).type = FieldType.other;
  }
}

function readCell(field: Field, i: number): unknown {
  const v = field.values as { get?: (i: number) => unknown } | unknown[] | undefined;
  if (v && typeof (v as { get?: unknown }).get === 'function') {
    return (v as { get: (i: number) => unknown }).get(i);
  }
  return (v as unknown[] | undefined)?.[i];
}

function parseArrayCell(cell: unknown): unknown[] {
  if (Array.isArray(cell)) {
    return cell;
  }
  if (typeof cell !== 'string' || !cell) {
    return [];
  }
  try {
    const parsed = JSON.parse(cell);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}
