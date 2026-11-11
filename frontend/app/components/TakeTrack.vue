<script setup lang="ts">
/**
 * The track: one take's envelope under a movable window.
 *
 * The waveform is drawn on a canvas because at any useful zoom it is one bar
 * per pixel, and a thousand elements to say that is a thousand elements too
 * many. Everything you can grab — cuts, the selection, the cues — is a real
 * element on top, because those need hit areas and titles.
 */
import type { Cue, Cut, Take } from '~/composables/useStudio'
import type { Proposal, Span } from '~/composables/useTakeEditor'

const props = defineProps<{
  take: Take
  /** Passed separately from the take so a drag can show a cut moving before
   *  the server has been told about it. */
  cuts: Cut[]
  view: Span
  time: number
  selection: Span | null
  proposals: Proposal[]
  /** Highlighted proposal, drawn brighter than the rest. */
  focus: Proposal | null
}>()

const emit = defineEmits<{
  seek: [seconds: number]
  select: [span: Span | null]
  view: [span: Span]
  /** A cut's edge was dragged; the index is into the take's own cuts array. */
  recut: [index: number, span: Span]
  uncut: [index: number]
  takeProposal: [p: Proposal]
}>()

const studio = useStudio()

const root = useTemplateRef<HTMLElement>('root')
const canvas = useTemplateRef<HTMLCanvasElement>('canvas')
const { width, height } = useElementSize(root)

const span = computed(() => Math.max(props.view.to - props.view.from, 1e-6))

/** A time in seconds as a percentage across the visible window. */
function at(seconds: number) {
  return ((seconds - props.view.from) / span.value) * 100
}
function widthOf(seconds: number) {
  return (seconds / span.value) * 100
}
/** Only what the window can show, so nothing is laid out a mile off screen. */
function visible(s: Span) {
  return s.to > props.view.from && s.from < props.view.to
}

function secondsAt(clientX: number) {
  const el = root.value
  if (!el) return 0
  const rect = el.getBoundingClientRect()
  const f = (clientX - rect.left) / Math.max(rect.width, 1)
  return Math.min(props.take.duration, Math.max(0, props.view.from + f * span.value))
}

/* ---------- the waveform ---------- */

function draw() {
  const el = canvas.value
  const ctx = el?.getContext('2d')
  if (!el || !ctx) return

  const dpr = window.devicePixelRatio || 1
  const w = Math.max(Math.round(width.value), 1)
  const h = Math.max(Math.round(height.value), 1)
  el.width = w * dpr
  el.height = h * dpr
  el.style.width = `${w}px`
  el.style.height = `${h}px`
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)

  const levels = props.take.levels
  if (!levels?.length) return

  // One column per pixel: walk the samples once and keep the loudest that
  // lands in each column. A peak that survives is a peak you can aim at.
  const peaks = new Float32Array(w)
  for (const l of levels) {
    if (l.t < props.view.from || l.t > props.view.to) continue
    const x = Math.min(w - 1, Math.floor(((l.t - props.view.from) / span.value) * w))
    if (l.r > peaks[x]!) peaks[x] = l.r
  }

  // Fill the gaps zoom leaves behind: past a certain magnification there are
  // more columns than samples, and an unfilled column would read as silence.
  let previous = 0
  for (let x = 0; x < w; x++) {
    if (peaks[x]! > 0) previous = peaks[x]!
    else if (span.value / w < 0.05) peaks[x] = previous
  }

  const mid = h / 2
  ctx.fillStyle = '#46525f'
  for (let x = 0; x < w; x++) {
    const amp = Math.min(1, peaks[x]! * 3) * (h / 2 - 2)
    ctx.fillRect(x, mid - Math.max(amp, 0.5), 1, Math.max(amp * 2, 1))
  }

  // The floor the silence detector works against, so a proposal is legible as
  // a consequence of something visible rather than a verdict out of nowhere.
  ctx.strokeStyle = 'rgba(255,255,255,.07)'
  ctx.beginPath()
  ctx.moveTo(0, mid)
  ctx.lineTo(w, mid)
  ctx.stroke()
}

watch([() => props.view, () => props.take.id, width, height], draw, { deep: true, flush: 'post' })
onMounted(draw)

/* ---------- the ruler ---------- */

/** A tick every 1, 5, 15, 30, 60… seconds, whichever keeps them readable. */
const ticks = computed(() => {
  const steps = [0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300, 600]
  const target = span.value / 8
  const step = steps.find(s => s >= target) ?? 600
  const out: { t: number; label: string }[] = []
  for (let t = Math.ceil(props.view.from / step) * step; t <= props.view.to; t += step) {
    out.push({ t, label: step < 1 ? `${t.toFixed(1)}s` : clock(t) })
  }
  return out
})

/* ---------- pointer grammar ---------- */

type Drag =
  | { mode: 'select'; anchor: number }
  | { mode: 'cut'; index: number; edge: 'from' | 'to'; other: number }
  | null

let drag: Drag = null
const dragging = ref(false)

function onDown(e: MouseEvent) {
  if (e.button !== 0) return
  const seconds = secondsAt(e.clientX)
  drag = { mode: 'select', anchor: seconds }
  dragging.value = true
  emit('select', null)
  emit('seek', seconds)
}

function grabCut(e: MouseEvent, index: number, edge: 'from' | 'to') {
  e.stopPropagation()
  const c = props.cuts[index]
  if (!c) return
  drag = { mode: 'cut', index, edge, other: edge === 'from' ? c.to : c.from }
  dragging.value = true
}

function onMove(e: MouseEvent) {
  if (!drag) {
    hover.value = secondsAt(e.clientX)
    return
  }
  const seconds = secondsAt(e.clientX)
  if (drag.mode === 'select') {
    if (Math.abs(seconds - drag.anchor) < span.value / 400) return
    emit('select', { from: Math.min(drag.anchor, seconds), to: Math.max(drag.anchor, seconds) })
    return
  }
  emit('recut', drag.index, {
    from: Math.min(drag.other, seconds),
    to: Math.max(drag.other, seconds),
  })
}

function onUp() {
  drag = null
  dragging.value = false
}

const hover = ref<number | null>(null)

function onWheel(e: WheelEvent) {
  e.preventDefault()
  if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
    const step = (e.deltaX / Math.max(width.value, 1)) * span.value
    emit('view', { from: props.view.from + step, to: props.view.to + step })
    return
  }
  const factor = e.deltaY > 0 ? 1.18 : 1 / 1.18
  const anchor = secondsAt(e.clientX)
  const f = (anchor - props.view.from) / span.value
  const next = span.value * factor
  emit('view', { from: anchor - f * next, to: anchor - f * next + next })
}

useEventListener(window, 'mouseup', onUp)
useEventListener(window, 'mousemove', (e: MouseEvent) => { if (drag) onMove(e) })

/* ---------- what is drawn on top ---------- */

const cutBands = computed(() =>
  props.cuts
    .map((c, index) => ({ c, index }))
    .filter(({ c }) => visible(c)),
)

/*
A quote occupies time, so it is drawn as the stretch it occupies.

While it plays the microphone hears nothing, so the envelope there is flat and
looks exactly like dead air. Marking only the instant it fired left no way to
tell the pause before a turn from the turn itself, which is precisely what you
are aiming at when you trim the gap after saying the character's name.
*/
const cueBands = computed(() =>
  (props.take.cues ?? [])
    .map((c) => {
      const quote = studio.quoteById(c.quote)
      return { cue: c, from: c.at, to: c.at + (quote?.dur ?? 0) }
    })
    .filter(b => visible(b)),
)

const proposalBands = computed(() => props.proposals.filter(visible))

function cueLabel(c: Cue) {
  const q = studio.quoteById(c.quote)
  return q ? `${c.quote} · ${q.personaje}` : c.quote
}
function cueColour(c: Cue) {
  return studio.colourOf(studio.quoteById(c.quote)?.personaje ?? '')
}
function cutLength(c: Cut) {
  return c.to - c.from
}
</script>

<template>
  <div
    ref="root"
    class="relative w-full h-full bg-[#0d0f12] border border-score-grid rounded-[3px] overflow-hidden select-none"
    :class="dragging ? 'cursor-col-resize' : 'cursor-text'"
    @mousedown="onDown"
    @mousemove="onMove"
    @mouseleave="hover = null"
    @wheel="onWheel"
  >
    <canvas ref="canvas" class="absolute inset-0 pointer-events-none" />

    <!-- ruler -->
    <div class="absolute inset-x-0 top-0 h-[15px] bg-score-bg/70 pointer-events-none">
      <template v-for="t in ticks" :key="t.t">
        <div class="absolute top-0 bottom-0 w-px bg-white/10" :style="{ left: `${at(t.t)}%` }" />
        <div
          class="absolute top-[1px] pl-1 text-[8.5px] tabular text-score-faint"
          :style="{ left: `${at(t.t)}%` }"
        >{{ t.label }}</div>
      </template>
    </div>

    <!-- what the reader already re-read, or never said: offered, not taken -->
    <div
      v-for="(p, i) in proposalBands"
      :key="`p${i}`"
      class="absolute top-[15px] bottom-[14px] rounded-[2px] border border-dashed transition-colors"
      :class="[
        p.kind === 'retake'
          ? 'border-[var(--color-voice-socrates)]/70 bg-[var(--color-voice-socrates)]/10 hover:bg-[var(--color-voice-socrates)]/20'
          : 'border-score-edge bg-white/[0.04] hover:bg-white/[0.08]',
        focus === p ? 'ring-1 ring-inset ring-white/40' : '',
      ]"
      :style="{ left: `${at(p.from)}%`, width: `${widthOf(p.to - p.from)}%` }"
      :title="`${p.why} — clic para cortar`"
      @mousedown.stop
      @click.stop="emit('takeProposal', p)"
    />

    <!-- cuts, with an edge you can take hold of -->
    <div
      v-for="{ c, index } in cutBands"
      :key="`c${index}`"
      class="absolute top-[15px] bottom-[14px] group"
      :style="{ left: `${at(c.from)}%`, width: `${widthOf(cutLength(c))}%` }"
    >
      <div
        class="absolute inset-0 border-x border-[var(--color-voice-zenon)]"
        style="background: repeating-linear-gradient(45deg, rgba(255,84,112,.24) 0 5px, transparent 5px 10px)"
        :title="`corte · ${cutLength(c).toFixed(2)} s`"
      />
      <div
        class="absolute inset-y-0 -left-[3px] w-[7px] cursor-col-resize hover:bg-[var(--color-voice-zenon)]/50"
        @mousedown="grabCut($event, index, 'from')"
      />
      <div
        class="absolute inset-y-0 -right-[3px] w-[7px] cursor-col-resize hover:bg-[var(--color-voice-zenon)]/50"
        @mousedown="grabCut($event, index, 'to')"
      />
      <button
        class="absolute -top-[1px] left-1/2 -translate-x-1/2 px-1 rounded-b-[2px] bg-[var(--color-voice-zenon)] text-score-bg text-[9px] leading-[13px] font-bold opacity-0 group-hover:opacity-100 transition-opacity"
        title="quitar el corte"
        @mousedown.stop
        @click.stop="emit('uncut', index)"
      >×</button>
    </div>

    <!-- the pending selection -->
    <div
      v-if="selection && visible(selection)"
      class="absolute top-[15px] bottom-[14px] bg-white/15 border-x border-white/50 pointer-events-none"
      :style="{ left: `${at(selection.from)}%`, width: `${widthOf(selection.to - selection.from)}%` }"
    />

    <!-- the cues: a quote is a hole in the microphone track, never a mistake.
         The band is the whole clip; the solid edge is where it starts. -->
    <div
      v-for="b in cueBands"
      :key="b.cue.quote"
      class="absolute bottom-[14px] top-[15px] pointer-events-none border-l-2"
      :style="{
        left: `${at(b.from)}%`,
        width: `${Math.max(widthOf(b.to - b.from), 0.3)}%`,
        borderColor: cueColour(b.cue),
        background: `color-mix(in srgb, ${cueColour(b.cue)} 22%, transparent)`,
      }"
      :title="`${cueLabel(b.cue)} · ${(b.to - b.from).toFixed(1)} s`"
    >
      <span
        class="absolute bottom-0 left-0 px-0.5 text-[8px] font-bold tabular text-score-bg rounded-t-[2px]"
        :style="{ background: cueColour(b.cue) }"
      >{{ b.cue.quote.slice(0, 2) }}</span>
    </div>

    <!-- the playhead -->
    <div
      class="absolute top-[15px] bottom-0 w-px bg-signal pointer-events-none z-10"
      style="box-shadow: 0 0 10px rgba(245,247,73,.6)"
      :style="{ left: `${at(time)}%` }"
    />
    <div
      v-if="hover !== null && !dragging"
      class="absolute top-[15px] bottom-[14px] w-px bg-white/25 pointer-events-none"
      :style="{ left: `${at(hover)}%` }"
    />

    <!-- the strip position, which is the coordinate the reader thinks in -->
    <div class="absolute inset-x-0 bottom-0 h-[14px] bg-score-stave/80 border-t border-score-grid pointer-events-none flex items-center px-1.5">
      <span class="text-[8.5px] tabular text-score-faint">
        {{ clock(view.from) }} – {{ clock(view.to) }}
        <span v-if="hover !== null" class="text-score-dim ml-2">
          · {{ hover.toFixed(2) }} s
        </span>
      </span>
    </div>

    <div
      v-if="!take.levels?.length"
      class="absolute inset-0 grid place-items-center text-[11px] text-score-faint"
    >Esta toma no guardó envolvente; se grabó antes de que existiera.</div>
  </div>
</template>
