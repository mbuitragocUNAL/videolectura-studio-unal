/**
 * Editing one take.
 *
 * Everything here is measured in seconds of the recording, the only clock a
 * take has. Two things make this more than a list of ranges:
 *
 *  - A **view window**. Without zoom the track is a picture, not an
 *    instrument: half an hour across a thousand pixels is two seconds a pixel,
 *    and nobody trims a stumble at two seconds a pixel.
 *  - **Proposals**. The take already knows where it went wrong. The scroll
 *    trace records every time the reader jumped back to re-read a passage, and
 *    the envelope records every silence. Both are cuts waiting to be made, so
 *    the studio offers them instead of making the reader hunt.
 */
import type { Cue, Cut, Take, TracePoint } from './useStudio'

/** The shortest window the track will zoom to: a quarter second, about a syllable. */
const MIN_WINDOW = 0.25

/**
 * A backward step in the scroll trace this small is jitter, not an intention.
 * The nudge key moves 200 px of a 21 856 px strip, so a real jump is two
 * orders of magnitude larger than this.
 */
const JITTER = 0.0001

export interface Proposal {
  from: number
  to: number
  kind: 'retake' | 'silence'
  why: string
}

export interface Span { from: number; to: number }

/** The stretches of a take that survive its cuts, in order. */
export function keptSpans(cuts: Cut[], duration: number): Span[] {
  const merged = mergeCuts(cuts)
  const spans: Span[] = []
  let cursor = 0
  for (const c of merged) {
    if (c.from > cursor) spans.push({ from: cursor, to: Math.min(c.from, duration) })
    if (c.to > cursor) cursor = c.to
  }
  if (cursor < duration) spans.push({ from: cursor, to: duration })
  return spans.filter(s => s.to - s.from > 0.02)
}

/** Sorted, disjoint cuts. Mirrors the server so the two never disagree. */
export function mergeCuts(cuts: Cut[]): Cut[] {
  const sorted = [...cuts].filter(c => c.to > c.from).sort((a, b) => a.from - b.from)
  const merged: Cut[] = []
  for (const c of sorted) {
    const last = merged[merged.length - 1]
    if (last && c.from <= last.to) last.to = Math.max(last.to, c.to)
    else merged.push({ ...c })
  }
  return merged
}

/** How much of the take the cuts remove. */
export function removedBy(cuts: Cut[]): number {
  return mergeCuts(cuts).reduce((s, c) => s + (c.to - c.from), 0)
}

export function useTakeEditor(take: Ref<Take | null>) {
  const studio = useStudio()
  const duration = computed(() => Math.max(take.value?.duration ?? 0, 0.001))
  const cuts = computed(() => take.value?.cuts ?? [])
  const cues = computed(() => take.value?.cues ?? [])

  /* ---------- the view window ---------- */

  const view = ref<Span>({ from: 0, to: 1 })

  /** A new take is shown whole; there is nothing to zoom into yet. */
  watch(() => take.value?.id, () => fit(), { immediate: true })
  watch(duration, () => { if (view.value.to <= view.value.from) fit() })

  function fit() {
    view.value = { from: 0, to: duration.value }
  }

  function clampView(from: number, to: number) {
    let span = Math.min(Math.max(to - from, MIN_WINDOW), duration.value)
    let start = Math.min(Math.max(from, 0), duration.value - span)
    if (!Number.isFinite(start) || start < 0) start = 0
    if (span > duration.value) span = duration.value
    view.value = { from: start, to: start + span }
  }

  /**
   * Zoom keeping one instant pinned where it already is on screen, so the
   * thing you are looking at does not slide out from under the pointer.
   */
  function zoomAround(seconds: number, factor: number) {
    const { from, to } = view.value
    const anchor = Math.min(Math.max((seconds - from) / Math.max(to - from, 1e-6), 0), 1)
    const span = (to - from) * factor
    clampView(seconds - anchor * span, seconds - anchor * span + span)
  }

  function zoomToSpan(span: Span, margin = 0.25) {
    const pad = Math.max((span.to - span.from) * margin, 0.2)
    clampView(span.from - pad, span.to + pad)
  }

  function pan(seconds: number) {
    clampView(view.value.from + seconds, view.value.to + seconds)
  }

  /** Keep an instant inside the window, scrolling the window if it left. */
  function reveal(seconds: number) {
    const { from, to } = view.value
    const span = to - from
    if (seconds >= from + span * 0.08 && seconds <= to - span * 0.08) return
    clampView(seconds - span / 2, seconds + span / 2)
  }

  const zoomed = computed(() => view.value.to - view.value.from < duration.value - 1e-6)

  /* ---------- the selection ---------- */

  const selection = ref<Span | null>(null)

  /**
   * Cues a pending selection would damage, unless the quote sounds again later.
   *
   * A quote needs the whole stretch it plays over, not just the instant it
   * starts. The microphone recorded silence there on purpose, and cutting any
   * of it shortens the hole the clip is mixed into, so the quote spills over
   * the words that follow. Guarding only the first instant let a cut that
   * began a moment later do exactly that.
   */
  const endangered = computed<Cue[]>(() => {
    const sel = selection.value
    if (!sel) return []
    return cues.value.filter((c) => {
      const ends = c.at + (studio.quoteById(c.quote)?.dur ?? 0)
      if (ends < sel.from || c.at > sel.to) return false
      // A quote fired twice — a flubbed pass and a good one — survives the cut
      // that removes the flub, because the good one is still there.
      return !cues.value.some(o => o.quote === c.quote && o.at > sel.to)
    })
  })

  /* ---------- proposals ---------- */

  /** Passages the reader went back and read again. See findRetakes. */
  const retakes = computed<Proposal[]>(() => findRetakes(take.value?.trace ?? []))

  /**
   * Stretches where the microphone heard nothing for long enough to be a
   * mistake rather than a breath.
   *
   * A quote is deliberate silence — it plays through the headphones so the mic
   * records a clean gap of exactly its length — so any quiet that overlaps a
   * cue is left strictly alone. Cutting one would delete the hole the quote
   * has to be mixed into.
   */
  const silences = computed<Proposal[]>(() => {
    const t = take.value
    const levels = t?.levels
    if (!t || !levels || levels.length < 20) return []

    const loud = [...levels].map(l => l.r).sort((a, b) => a - b)
    const ceiling = loud[Math.floor(loud.length * 0.9)] ?? 0
    const floor = Math.max(ceiling * 0.06, 0.008)

    // Where a quote is sounding, plus a little air on each side.
    const protectedRanges = cues.value.map((c) => {
      const dur = studio.quoteById(c.quote)?.dur ?? 0
      return { from: c.at - 0.4, to: c.at + dur + 0.6 }
    })

    const found: Proposal[] = []
    let start: number | null = null
    for (let i = 0; i < levels.length; i++) {
      const quiet = levels[i]!.r < floor
      if (quiet && start === null) start = levels[i]!.t
      if (!quiet && start !== null) {
        pushSilence(found, start, levels[i]!.t, protectedRanges)
        start = null
      }
    }
    if (start !== null) pushSilence(found, start, t.duration, protectedRanges)
    return mergeProposals(found)
  })

  const proposals = computed(() => {
    const applied = mergeCuts(cuts.value)
    const already = (p: Proposal) =>
      applied.some(c => p.from >= c.from - 0.05 && p.to <= c.to + 0.05)
    return [...retakes.value, ...silences.value]
      .filter(p => !already(p))
      .sort((a, b) => a.from - b.from)
  })

  /* ---------- playback over the cuts ---------- */

  /**
   * Where playback should jump to if it has wandered into a cut, so previewing
   * plays the montage rather than the raw take.
   */
  function skipCuts(seconds: number): number | null {
    for (const c of mergeCuts(cuts.value)) {
      if (seconds >= c.from && seconds < c.to) return c.to
    }
    return null
  }

  /** The same instant measured on the finished piece rather than the take. */
  function netTime(seconds: number): number {
    let net = seconds
    for (const c of mergeCuts(cuts.value)) {
      if (c.to <= seconds) net -= c.to - c.from
      else if (c.from < seconds) net -= seconds - c.from
    }
    return Math.max(0, net)
  }

  const removed = computed(() => removedBy(cuts.value))
  const kept = computed(() => Math.max(0, duration.value - removed.value))

  return {
    duration, cuts, cues, kept, removed,
    view, zoomed, fit, clampView, zoomAround, zoomToSpan, pan, reveal,
    selection, endangered,
    retakes, silences, proposals,
    skipCuts, netTime,
  }
}

/**
 * Passages the reader went back and read again.
 *
 * When a reading goes wrong the reader nudges the strip backwards and starts
 * the sentence over. The trace shows that as the only thing it ever does that
 * is not monotonic: a step backwards. Everything between the moment the strip
 * first reached the point it landed on and the moment it jumped back is the
 * bad pass, and that is exactly what wants removing.
 *
 * Exported and pure so it can be tested without a take, a component or a
 * browser — there is no other way to exercise it until someone fluffs a line.
 */
export function findRetakes(trace: TracePoint[]): Proposal[] {
  if (trace.length < 3) return []

  const found: Proposal[] = []
  for (let i = 1; i < trace.length; i++) {
    const here = trace[i]!
    const before = trace[i - 1]!
    if (here.f >= before.f - JITTER) continue

    // Walk back to when the strip first passed the point it landed on.
    let j = i - 1
    while (j > 0 && trace[j - 1]!.f >= here.f) j--

    const from = trace[j]!.t
    const to = before.t
    if (to - from < 0.4) continue
    found.push({
      from,
      to,
      kind: 'retake',
      why: 'volviste atrás aquí: esta pasada se repite después',
    })
  }
  return mergeProposals(found)
}

function pushSilence(into: Proposal[], from: number, to: number, protect: Span[]) {
  if (to - from < 1.2) return
  if (protect.some(p => from < p.to && to > p.from)) return
  // Leave a quarter second of room at each end: a splice flush against speech
  // clips the breath before the word and sounds wrong even when the timing is
  // right.
  const a = from + 0.25
  const b = to - 0.25
  if (b - a < 0.5) return
  into.push({
    from: a,
    to: b,
    kind: 'silence',
    why: `${(b - a).toFixed(1)} s de silencio`,
  })
}

function mergeProposals(list: Proposal[]): Proposal[] {
  const sorted = [...list].sort((a, b) => a.from - b.from)
  const out: Proposal[] = []
  for (const p of sorted) {
    const last = out[out.length - 1]
    if (last && p.from <= last.to + 0.15) {
      last.to = Math.max(last.to, p.to)
      continue
    }
    out.push({ ...p })
  }
  return out
}
