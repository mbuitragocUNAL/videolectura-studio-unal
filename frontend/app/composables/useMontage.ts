/**
 * The montage as it will be delivered: every take in the order the passage
 * runs, with its cuts already taken out.
 *
 * The render is the truth, but the render is minutes away. This is the same
 * arithmetic the Go side does — OrderedTakes over KeptSpans — done in the
 * browser so the assembled reading can be watched now, at the point where a
 * splice is still cheap to fix.
 */
import type { Take } from './useStudio'

/** One stretch of one take, placed on the finished video's clock. */
export interface Segment {
  takeId: number
  label: string
  paragraph: number
  /** Seconds within the take's own recording. */
  from: number
  to: number
  /** Second of the finished video this stretch begins at. */
  startsAt: number
}

export const useMontage = () => {
  const studio = useStudio()

  /**
   * Takes in the order the passage runs, not the order they were recorded.
   * A paragraph read again after a stumble gets a later id but belongs where
   * its text is; this mirrors the server so the preview cannot disagree with
   * the render.
   */
  const ordered = computed<Take[]>(() => {
    // One take per slot — the newest — and nothing that has been cut away
    // entirely. The same rules the server applies, so the preview and the
    // render cannot disagree about what the video contains.
    //
    // A slot is a paragraph *and* whether it is the lámina, not the paragraph
    // alone. Keyed on the paragraph by itself, the plate and the reading of
    // the paragraph that announces it collided, the plate won on id, and the
    // reading vanished from the preview while the render kept both.
    const newest = new Map<string, Take>()
    for (const take of studio.takes.value) {
      // And which figure: a paragraph can name two.
      const slot = `${take.paragraph}:${take.plate ? `lamina:${take.figure ?? ''}` : 'lectura'}`
      const seen = newest.get(slot)
      if (!seen || take.id > seen.id) newest.set(slot, take)
    }
    return [...newest.values()]
      .filter(t => t.duration - removedBy(t.cuts ?? []) > 0)
      .sort((a, b) => {
        if (a.paragraph !== b.paragraph) return a.paragraph - b.paragraph
        // The plate follows the paragraph that announces it.
        if (a.plate !== b.plate) return a.plate ? 1 : -1
        if (a.from !== b.from) return a.from - b.from
        return a.id - b.id
      })
  })

  /** Takes kept on disk but left out of the video, so they can be cleaned up. */
  const superseded = computed(() => {
    const used = new Set(ordered.value.map(t => t.id))
    return studio.takes.value.filter(t => !used.has(t.id))
  })

  const segments = computed<Segment[]>(() => {
    const out: Segment[] = []
    let clock = 0
    for (const take of ordered.value) {
      for (const span of keptSpans(take.cuts ?? [], take.duration)) {
        out.push({
          takeId: take.id,
          label: take.label,
          paragraph: take.paragraph,
          from: span.from,
          to: span.to,
          startsAt: clock,
        })
        clock += span.to - span.from
      }
    }
    return out
  })

  const total = computed(() =>
    segments.value.reduce((s, x) => s + (x.to - x.from), 0),
  )

  /** The segment playing at a second of the finished video. */
  function segmentAt(seconds: number): Segment | null {
    const list = segments.value
    if (!list.length) return null
    let lo = 0
    let hi = list.length - 1
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2)
      if (list[mid]!.startsAt <= seconds) lo = mid
      else hi = mid - 1
    }
    return list[lo]!
  }

  /** Where in its own take a second of the finished video lands. */
  function sourceTime(seconds: number) {
    const segment = segmentAt(seconds)
    if (!segment) return null
    return { segment, at: segment.from + (seconds - segment.startsAt) }
  }

  /**
   * Paragraphs with no take. The one number that says whether the reading is
   * finished, and the list that says what is left.
   *
   * The tail paragraphs of a quote are not counted: they are the same clip
   * continuing, already covered by the take of the paragraph it starts in.
   */
  const missing = computed(() =>
    studio.paragraphs.value.filter(
      p => !studio.quoteTails.value.has(p.index) && !studio.takeOfParagraph(p.index),
    ),
  )

  return { ordered, superseded, segments, total, segmentAt, sourceTime, missing }
}
