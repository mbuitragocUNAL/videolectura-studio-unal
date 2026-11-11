<script setup lang="ts">
/**
 * The score: every stave shares one x-axis and one playhead crosses all of
 * them. The axis is position in the reading (0..1 of the strip), not wall
 * time, because that is the only coordinate takes, quotes, sections and lines
 * all agree on.
 *
 * It zooms. A reading of seventy-nine paragraphs puts seventy-nine take blocks
 * on a lane a thousand pixels wide — thirteen pixels each, which is a texture
 * rather than a thing you can point at. The window is in the same strip
 * coordinate as everything else, so zooming in is just narrowing it.
 */
const props = defineProps<{
  /** Playhead position, 0..1 of the strip. */
  head: number
  /** Total runtime, used only to label the ruler. */
  seconds: number
}>()

const emit = defineEmits<{ seek: [f: number]; pick: [takeId: number] }>()

const studio = useStudio()
const { sections, quotes, takes } = studio

const lanes = useTemplateRef<HTMLElement>('lanes')
const { width: laneWidth } = useElementSize(lanes)

/* ---------- the window ---------- */

/** The narrowest view: about one paragraph of a seventy-nine paragraph reading. */
const MIN_SPAN = 0.008

const view = ref({ from: 0, to: 1 })
const span = computed(() => Math.max(view.value.to - view.value.from, 1e-6))
const zoomed = computed(() => span.value < 1 - 1e-6)

function setView(from: number, to: number) {
  let width = Math.min(Math.max(to - from, MIN_SPAN), 1)
  const start = Math.min(Math.max(from, 0), 1 - width)
  if (!Number.isFinite(start)) return
  if (width > 1) width = 1
  view.value = { from: Math.max(0, start), to: Math.min(1, start + width) }
}

const fit = () => setView(0, 1)

/** Zoom keeping the point under the pointer where it already is. */
function zoomAround(f: number, factor: number) {
  const held = (f - view.value.from) / span.value
  const next = span.value * factor
  setView(f - held * next, f - held * next + next)
}

/** Keep the playhead in view while it travels, but never fight a manual pan. */
watch(() => props.head, (f) => {
  if (!zoomed.value || scrubbing) return
  const { from, to } = view.value
  if (f >= from + span.value * 0.05 && f <= to - span.value * 0.05) return
  setView(f - span.value / 2, f + span.value / 2)
})

/* ---------- mapping strip position to the lane ---------- */

/** A strip position as a percentage across the visible window. */
const at = (f: number) => ((f - view.value.from) / span.value) * 100
const widthOf = (f: number) => (f / span.value) * 100
const visible = (from: number, to: number) => to >= view.value.from && from <= view.value.to

function positionAt(clientX: number) {
  const el = lanes.value
  if (!el) return 0
  const rect = el.getBoundingClientRect()
  const local = (clientX - rect.left) / Math.max(rect.width, 1)
  return Math.min(1, Math.max(0, view.value.from + local * span.value))
}

/* ---------- pointer ---------- */

const hovering = ref<number | null>(null)
let scrubbing = false
let panning: { clientX: number; from: number } | null = null

function onLaneDown(e: MouseEvent) {
  if (e.button === 1 || e.shiftKey) {
    panning = { clientX: e.clientX, from: view.value.from }
    e.preventDefault()
    return
  }
  if (e.button !== 0) return
  scrubbing = true
  const f = positionAt(e.clientX)
  hovering.value = f
  emit('seek', f)
}

function onLaneMove(e: MouseEvent) {
  if (panning) {
    const moved = (e.clientX - panning.clientX) / Math.max(laneWidth.value, 1)
    setView(panning.from - moved * span.value, panning.from - moved * span.value + span.value)
    return
  }
  const f = positionAt(e.clientX)
  hovering.value = f
  if (scrubbing) emit('seek', f)
}

function onWheel(e: WheelEvent) {
  if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
    e.preventDefault()
    const step = (e.deltaX / Math.max(laneWidth.value, 1)) * span.value
    setView(view.value.from + step, view.value.to + step)
    return
  }
  e.preventDefault()
  zoomAround(positionAt(e.clientX), e.deltaY > 0 ? 1.2 : 1 / 1.2)
}

useEventListener(window, 'mouseup', () => { scrubbing = false; panning = null })
useEventListener(window, 'mousemove', (e: MouseEvent) => {
  if (scrubbing || panning) onLaneMove(e)
})

/** The line under the pointer, so a position is a place in the text. */
const hoveredLine = computed(() =>
  hovering.value === null ? null : studio.lineAt(hovering.value),
)

/* ---------- what is drawn ---------- */

/**
 * Ruler marks at a round fraction of the strip, spaced so they stay readable
 * however far in the window is. The labels are minutes of the projection.
 */
const ruler = computed(() => {
  const steps = [0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5]
  const step = steps.find(s => s >= span.value / 9) ?? 0.5
  const out: { f: number; label: string }[] = []
  for (let f = Math.ceil(view.value.from / step) * step; f <= view.value.to + 1e-9; f += step) {
    out.push({ f: Math.min(1, f), label: clock(props.seconds * f) })
  }
  return out
})

/** Sections laid end to end: each runs until the next one begins. */
const sectionSpans = computed(() =>
  sections.value
    .map((s, i) => ({ ...s, end: sections.value[i + 1]?.f ?? 1 }))
    .filter(s => visible(s.f, s.end)),
)

/**
 * Takes are placed by the stretch of the strip they were recorded over, so a
 * take sits under the text it was reading. A take with no span recorded falls
 * back to a thin marker rather than vanishing.
 */
const takeBlocks = computed(() =>
  takes.value
    .map((t) => {
      const from = t.from ?? 0
      const to = Math.max(t.to ?? 0, from + 0.0015)
      const removed = (t.cuts ?? []).reduce((s, c) => s + (c.to - c.from), 0)
      return {
        take: t,
        from,
        to,
        cuts: (t.cuts ?? []).map(c => ({
          left: (c.from / Math.max(t.duration, 0.001)) * 100,
          width: ((c.to - c.from) / Math.max(t.duration, 0.001)) * 100,
        })),
        kept: Math.max(0, t.duration - removed),
      }
    })
    .filter(b => visible(b.from, b.to)),
)

/** Quotes that own a block of the strip get a bar; inline ones get a tick. */
const quoteBlocks = computed(() =>
  quotes.value
    .filter(q => q.fFin !== null)
    .map(q => ({ quote: q, from: q.f, to: q.fFin as number }))
    .filter(b => visible(b.from, b.to)),
)

/**
 * One bracket per exchange rather than one tick per quote. The 02-06 dialogue
 * is five turns inside a hair's width of strip; as separate marks they collide
 * into an unreadable clump precisely where the reader most needs to see what
 * is coming.
 */
const groupBrackets = computed(() =>
  studio.groups.value
    .map((g, i) => {
      const first = studio.quoteById(g.clips[0]!)
      const last = studio.quoteById(g.clips[g.clips.length - 1]!)
      const from = first?.f ?? 0
      const to = last?.fFin ?? last?.f ?? 0
      return {
        key: g.clips.join('+'),
        from,
        to,
        turns: g.clips.map(id => studio.colourOf(studio.quoteById(id)?.personaje ?? '')),
        single: g.clips.length === 1,
        title: g.clips.length > 1
          ? `${g.clips.length} turnos · ${studio.groupDuration(i).toFixed(1)} s`
          : `${first?.personaje} · ${first?.dur.toFixed(1)} s`,
      }
    })
    .filter(b => visible(b.from, b.to)),
)

/** Bars narrower than this would be invisible, so they are drawn as marks. */
const leastWidth = (f: number) => Math.max(widthOf(f), 0.25)
</script>

<template>
  <div class="relative select-none">
    <!-- What the pointer is over, named in the coordinate the reader thinks
         in. Without it the ruler is a bar of minutes, and minutes are not
         where anybody wants to start a take. -->
    <div
      v-if="hoveredLine && hovering !== null"
      class="absolute -top-[15px] z-20 pointer-events-none flex items-baseline gap-1.5 text-[10px] whitespace-nowrap"
      :style="{
        left: `calc(104px + ${at(hovering) / 100} * (100% - 104px))`,
        transform: at(hovering) > 75 ? 'translateX(-100%)' : 'none',
      }"
    >
      <span class="tabular text-signal">línea {{ hoveredLine.n }}</span>
      <span class="text-score-dim max-w-[380px] truncate">{{ hoveredLine.t.trim() }}</span>
    </div>

    <div
      v-if="zoomed"
      class="absolute -top-[15px] right-0 z-20 flex items-center gap-2 text-[10px] text-score-faint"
    >
      <span class="tabular">{{ (span * 100).toFixed(1) }}% de la tira</span>
      <button class="text-score-dim hover:text-white underline underline-offset-2" @click="fit()">
        ver todo
      </button>
    </div>

    <!-- The one playhead. It is positioned against the lanes, which start
         after the clef column, so it lines up on every stave. -->
    <div
      v-if="at(head) >= 0 && at(head) <= 100"
      class="absolute top-0 bottom-0 w-px bg-signal z-10 pointer-events-none"
      style="box-shadow: 0 0 12px rgba(245, 247, 73, .55)"
      :style="{ left: `calc(104px + ${at(head) / 100} * (100% - 104px))` }"
    >
      <div
        class="absolute -top-[5px] -left-1 w-0 h-0"
        style="border-left:4.5px solid transparent;border-right:4.5px solid transparent;border-top:6px solid var(--color-signal)"
      />
    </div>

    <ScoreStave name="" :height="26" bare>
      <div
        ref="lanes"
        class="absolute inset-0 cursor-col-resize"
        @mousedown="onLaneDown"
        @mousemove="onLaneMove"
        @mouseleave="hovering = null"
        @wheel="onWheel"
      >
        <template v-for="m in ruler" :key="m.f">
          <div class="absolute top-[7px] bottom-0 w-px bg-score-faint" :style="{ left: `${at(m.f)}%` }" />
          <div
            class="absolute top-0 -translate-x-1/2 text-[9px] text-score-faint tabular"
            :style="{ left: `${at(m.f)}%` }"
          >{{ m.label }}</div>
        </template>
      </div>
    </ScoreStave>

    <ScoreStave name="Secciones" :meta="`${sections.length}`" :height="30">
      <div
        v-for="s in sectionSpans"
        :key="s.linea"
        class="absolute top-[3px] bottom-[3px] border-l-2 pl-[7px] flex items-center overflow-hidden whitespace-nowrap text-[10.5px]"
        :class="head >= s.f && head < s.end
          ? 'border-signal text-white bg-signal/5'
          : 'border-score-edge text-score-dim'"
        :style="{ left: `${at(s.f)}%`, width: `${widthOf(s.end - s.f)}%` }"
        :title="s.titulo"
      >{{ s.titulo }}</div>
    </ScoreStave>

    <ScoreStave
      name="Voz"
      :meta="takes.length ? `${takes.length} tomas · ${clock(studio.keptSeconds.value)}` : 'sin tomas'"
      :height="62"
    >
      <div
        v-for="b in takeBlocks"
        :key="b.take.id"
        class="absolute top-[5px] bottom-[5px] rounded-[2px] bg-[#1d232b] border border-[#2a323c] overflow-hidden cursor-pointer hover:border-score-faint"
        :style="{ left: `${at(b.from)}%`, width: `${leastWidth(b.to - b.from)}%` }"
        :title="`${b.take.label || 'toma ' + b.take.id} · ${clock(b.kept)}`"
        @mousedown.stop
        @click.stop="emit('pick', b.take.id)"
      >
        <span
          v-if="widthOf(b.to - b.from) > 4"
          class="absolute left-1.5 top-1 text-[9px] tabular text-score-dim bg-score-bg/70 px-1 rounded-[2px] max-w-[calc(100%-12px)] truncate"
        >{{ b.take.label || `t${b.take.id}` }}</span>
        <div
          v-for="(c, i) in b.cuts"
          :key="i"
          class="absolute top-0 bottom-0 border-x border-[var(--color-voice-zenon)]"
          style="background: repeating-linear-gradient(45deg, rgba(255,84,112,.2) 0 4px, transparent 4px 8px)"
          :style="{ left: `${c.left}%`, width: `${c.width}%` }"
        />
      </div>

      <div
        v-if="!takes.length"
        class="absolute inset-0 grid place-items-center text-[11px] text-score-faint"
      >Aún no hay tomas. Pulsa grabar para empezar la primera.</div>
    </ScoreStave>

    <ScoreStave
      name="Citas"
      :meta="`${quotes.length} · ${clock(quotes.reduce((s, q) => s + q.dur, 0))}`"
      :height="34"
    >
      <div
        v-for="b in quoteBlocks"
        :key="b.quote.clip"
        class="absolute top-[5px] bottom-[5px] rounded-[2px] flex items-center px-1.5 text-[9.5px] font-semibold tabular text-score-bg overflow-hidden whitespace-nowrap"
        :style="{
          left: `${at(b.from)}%`,
          width: `${leastWidth(b.to - b.from)}%`,
          background: studio.colourOf(b.quote.personaje),
        }"
        :title="`${b.quote.personaje} · ${b.quote.dur.toFixed(1)} s`"
      >{{ b.quote.clip.slice(0, 2) }}</div>
    </ScoreStave>

    <ScoreStave name="Diálogos" :meta="`${groupBrackets.length} grupos`" :height="24">
      <div
        v-for="g in groupBrackets"
        :key="g.key"
        class="absolute top-[5px] h-[13px] rounded-[2px] flex items-stretch gap-px overflow-hidden"
        :class="g.single ? '' : 'ring-1 ring-inset ring-white/15'"
        :style="{ left: `${at(g.from)}%`, width: `${leastWidth(g.to - g.from)}%` }"
        :title="g.title"
      >
        <span
          v-for="(turn, i) in g.turns"
          :key="i"
          class="flex-1 min-w-[2px]"
          :style="{ background: turn }"
        />
      </div>
    </ScoreStave>
  </div>
</template>
