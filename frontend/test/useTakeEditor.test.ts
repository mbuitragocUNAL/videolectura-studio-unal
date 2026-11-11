/**
 * Tests for the parts of the editor that are pure arithmetic over a take.
 *
 * They run under node's own test runner, which strips the types and imports
 * the composable directly. They live outside app/ because Nuxt auto-imports
 * everything in composables/, and a file importing node:test from the client
 * bundle would not survive the build. That works because everything tested here is declared
 * at module level and touches neither Vue nor the browser — which is also why
 * it was worth pulling these out of the composable in the first place. The
 * retake detector especially: it cannot be exercised by hand until somebody
 * fluffs a line on camera, so the only honest way to ship it is against traces
 * written on purpose.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { findRetakes, keptSpans, mergeCuts, removedBy } from '../app/composables/useTakeEditor.ts'
import type { TracePoint } from '../app/composables/useStudio.ts'

/** A strip creeping forward at 0.001 per sample, sampled at 20 Hz. */
function ramp(samples: number, start = 0, f0 = 0, step = 0.001): TracePoint[] {
  return Array.from({ length: samples }, (_, i) => ({
    t: Number((start + i * 0.05).toFixed(2)),
    f: Number((f0 + i * step).toFixed(6)),
  }))
}

const spans = (list: { from: number; to: number }[]) =>
  list.map(p => [Number(p.from.toFixed(2)), Number(p.to.toFixed(2))])

test('a clean pass proposes nothing', () => {
  assert.deepEqual(spans(findRetakes(ramp(200))), [])
})

test('a jump back proposes the pass that was repeated', () => {
  // Reads to 3 s, jumps back to the text it was on at 1 s, reads on.
  const trace = [...ramp(60), ...ramp(60, 3.0, 0.020)]
  assert.deepEqual(spans(findRetakes(trace)), [[1.0, 2.95]])
})

test('a step back too small to be meant is not a retake', () => {
  const trace = ramp(100).map((p, i) => (i === 50 ? { ...p, f: p.f - 0.00005 } : p))
  assert.deepEqual(spans(findRetakes(trace)), [])
})

test('a jump back over less than 0.4 s is not worth a cut', () => {
  const trace = [...ramp(6), ...ramp(40, 0.3, 0.003)]
  assert.deepEqual(spans(findRetakes(trace)), [])
})

test('two stumbles far apart stay two proposals', () => {
  const trace = [...ramp(40), ...ramp(40, 2.0, 0.010), ...ramp(40, 4.0, 0.030)]
  assert.equal(findRetakes(trace).length, 2)
})

test('a trace too short to read is safe', () => {
  assert.deepEqual(findRetakes([]), [])
  assert.deepEqual(findRetakes([{ t: 0, f: 0 }, { t: 1, f: 0.1 }]), [])
})

test('overlapping cuts merge into one', () => {
  assert.deepEqual(
    mergeCuts([{ from: 4, to: 6 }, { from: 1, to: 3 }, { from: 2, to: 5 }]),
    [{ from: 1, to: 6 }],
  )
})

test('a cut of no length is not a cut', () => {
  assert.deepEqual(mergeCuts([{ from: 2, to: 2 }, { from: 5, to: 4 }]), [])
  assert.equal(removedBy([{ from: 1, to: 3 }, { from: 2, to: 5 }]), 4)
})

test('what survives the cuts is their complement', () => {
  assert.deepEqual(
    keptSpans([{ from: 2, to: 4 }], 10),
    [{ from: 0, to: 2 }, { from: 4, to: 10 }],
  )
  // A sliver left behind by a cut that reaches the end is not a span.
  assert.deepEqual(keptSpans([{ from: 0, to: 9.99 }], 10), [])
  assert.deepEqual(keptSpans([], 10), [{ from: 0, to: 10 }])
})
